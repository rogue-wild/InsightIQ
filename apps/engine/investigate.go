package main

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

type Alert struct {
	ID           string  `json:"id"`
	Metric       string  `json:"metric"`
	Direction    string  `json:"direction"`
	PctChange    float64 `json:"pctChange"`
	WindowStart  string  `json:"windowStart"`
	WindowEnd    string  `json:"windowEnd"`
	BaselineKind string  `json:"baselineKind"`
	Severity     string  `json:"severity"`
	AdvertiserID string  `json:"advertiserId,omitempty"`
}

type Factor struct {
	Factor   string  `json:"factor"`
	Label    string  `json:"label"`
	Status   string  `json:"status"`
	Baseline float64 `json:"baseline"`
	Observed float64 `json:"observed"`
	DeltaPct float64 `json:"deltaPct"`
}

type Segment struct {
	Dimension       string  `json:"dimension"`
	Value           string  `json:"value"`
	Metric          string  `json:"metric"`
	DeltaPct        float64 `json:"deltaPct"`
	ContributionPct float64 `json:"contributionPct"`
}

type RuledOut struct {
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

type Citation struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Diagnosis struct {
	Text      string     `json:"text"`
	Citations []Citation `json:"citations"`
}

type TraceStep struct {
	Step       string `json:"step"`
	Detail     string `json:"detail"`
	DurationMs int64  `json:"durationMs"`
}

type Investigation struct {
	ID            string      `json:"id"`
	Status        string      `json:"status"`
	Alert         Alert       `json:"alert"`
	Decomposition []Factor    `json:"decomposition"`
	Segments      []Segment   `json:"segments"`
	RuledOut      []RuledOut  `json:"ruledOut"`
	Diagnosis     Diagnosis   `json:"diagnosis"`
	Trace         []TraceStep `json:"trace"`
}

type metricBag struct {
	Requests    float64
	Fills       float64
	Impressions float64
	Clicks      float64
	Revenue     float64
}

func (m metricBag) FillRate() float64 {
	if m.Requests == 0 {
		return 0
	}
	return m.Fills / m.Requests
}
func (m metricBag) RenderRate() float64 {
	if m.Fills == 0 {
		return 0
	}
	return m.Impressions / m.Fills
}
func (m metricBag) CTR() float64 {
	if m.Impressions == 0 {
		return 0
	}
	return m.Clicks / m.Impressions
}
func (m metricBag) ECPM() float64 {
	if m.Impressions == 0 {
		return 0
	}
	return m.Revenue / m.Impressions * 1000
}
func (m metricBag) RPR() float64 {
	if m.Requests == 0 {
		return 0
	}
	return m.Revenue / m.Requests
}

type InvestigateRequest struct {
	AlertID      string `json:"alertId"`
	Metric       string `json:"metric"`
	WindowStart  string `json:"windowStart"`
	WindowEnd    string `json:"windowEnd"`
	BaselineKind string `json:"baselineKind"`
	AdvertiserID string `json:"advertiserId"`
}

type alertLiveRow struct {
	AlertID      string
	AdvertiserID string
	Metric       string
	Bucket       time.Time
	Actual       float64
	Expected     float64
	ZScore       float64
}

func runInvestigation(ctx context.Context, conn *chClient, req InvestigateRequest) (*Investigation, error) {
	trace := []TraceStep{}
	mark := func(step, detail string, started time.Time) {
		trace = append(trace, TraceStep{Step: step, Detail: detail, DurationMs: time.Since(started).Milliseconds()})
	}

	t0 := time.Now()
	alertID := normalizeAlertID(req.AlertID)
	var live *alertLiveRow
	var err error
	if alertID != "" {
		live, err = fetchAlertLive(ctx, conn, alertID)
		if err != nil {
			return nil, err
		}
	}

	var windowStart, windowEnd time.Time
	metric := req.Metric
	advertiserID := req.AdvertiserID
	baselineKind := req.BaselineKind
	if baselineKind == "" {
		baselineKind = "same_hour_4w_seasonality"
	}

	if live != nil {
		windowStart = live.Bucket
		windowEnd = live.Bucket.Add(time.Hour - time.Second)
		if metric == "" {
			metric = live.Metric
		}
		if advertiserID == "" {
			advertiserID = live.AdvertiserID
		}
		alertID = live.AlertID
		mark("alerts_live", fmt.Sprintf("Loaded alert %s advertiser=%s metric=%s z=%.2f",
			alertID, advertiserID, metric, live.ZScore), t0)
	} else {
		windowStart, windowEnd, err = resolveWindow(req.WindowStart, req.WindowEnd)
		if err != nil {
			return nil, err
		}
		if metric == "" {
			metric = "revenue"
		}
		mark("window", fmt.Sprintf("No alert id; using window %s..%s",
			windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339)), t0)
	}

	t1 := time.Now()
	observed, err := querySnapshotMetrics(ctx, conn, advertiserID, windowStart, windowEnd)
	if err != nil {
		return nil, fmt.Errorf("observed metrics: %w", err)
	}
	baseline, err := querySeasonalBaseline(ctx, conn, advertiserID, windowStart, windowEnd, 4)
	if err != nil {
		return nil, fmt.Errorf("baseline metrics: %w", err)
	}
	if live != nil && metric == "revenue" && live.Expected > 0 && baseline.Revenue == 0 {
		baseline.Revenue = live.Expected
	}
	mark("baseline", fmt.Sprintf("metric_hourly_snapshot vs %s", baselineKind), t1)

	t2 := time.Now()
	decomp := decompose(baseline, observed)
	culprit := pickCulprit(decomp, metric)
	mark("decompose", fmt.Sprintf("Revenue identity walk; culprit=%s", culprit), t2)

	t3 := time.Now()
	segments, err := fetchContributors(ctx, conn, alertID, metric)
	if err != nil {
		return nil, fmt.Errorf("contributors: %w", err)
	}
	if len(segments) == 0 {
		segments, err = rankSegmentsFromSnapshot(ctx, conn, advertiserID, windowStart, windowEnd, culprit)
		if err != nil {
			return nil, fmt.Errorf("segments: %w", err)
		}
		mark("slice", fmt.Sprintf("Ranked dimensions from metric_hourly_snapshot (%d)", len(segments)), t3)
	} else {
		mark("slice", fmt.Sprintf("Loaded alert_dimension_contributors (%d)", len(segments)), t3)
	}

	t4 := time.Now()
	var pct float64
	if live != nil && live.Expected != 0 {
		pct = (live.Actual - live.Expected) / math.Abs(live.Expected) * 100
	} else {
		pct = pctChange(metricValue(observed, metric), metricValue(baseline, metric))
	}
	direction := "down"
	if pct > 0 {
		direction = "up"
	}
	if alertID == "" {
		alertID = fmt.Sprintf("alert-%s-%s", metric, windowStart.Format("2006-01-02"))
	}
	invID := "inv-" + alertID
	alert := Alert{
		ID: alertID, Metric: metric, Direction: direction, PctChange: round1(pct),
		WindowStart:  windowStart.UTC().Format(time.RFC3339),
		WindowEnd:    windowEnd.UTC().Format(time.RFC3339),
		BaselineKind: baselineKind, Severity: severityFromZOrPct(live, math.Abs(pct)),
		AdvertiserID: advertiserID,
	}

	observations, err := fetchObservations(ctx, conn, alertID)
	if err != nil {
		return nil, fmt.Errorf("observations: %w", err)
	}
	ruledOut := buildRuledOut(culprit, baseline, observed, pct)
	diagnosis := buildDiagnosisFromInsightIQ(alert, decomp, culprit, segments, ruledOut, observations, live)
	mark("evidence", "Packaged evidence from InsightIQ view layer", t4)
	trace = append(trace, TraceStep{Step: "narrate", Detail: "Narration deferred to Node/Gemini from evidence only", DurationMs: 0})

	return &Investigation{
		ID: invID, Status: "complete", Alert: alert,
		Decomposition: decomp, Segments: segments, RuledOut: ruledOut,
		Diagnosis: diagnosis, Trace: trace,
	}, nil
}

func normalizeAlertID(id string) string {
	id = strings.TrimSpace(id)
	id = strings.TrimPrefix(id, "inv-")
	return id
}

func fetchAlertLive(ctx context.Context, conn *chClient, alertID string) (*alertLiveRow, error) {
	q := fmt.Sprintf(`
		SELECT
			toString(alert_id) AS id,
			advertiser_id,
			metric,
			toString(bucket) AS bucket,
			actual,
			expected,
			zscore
		FROM alerts_live
		WHERE alert_id = toUUID(%s)
		LIMIT 1
	`, quoteString(alertID))
	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("alert not found: %s", alertID)
	}
	r := rows[0]
	bucket, err := parseCHTime(asString(r["bucket"]))
	if err != nil {
		return nil, err
	}
	return &alertLiveRow{
		AlertID: asString(r["id"]), AdvertiserID: asString(r["advertiser_id"]),
		Metric: asString(r["metric"]), Bucket: bucket,
		Actual: asFloat(r["actual"]), Expected: asFloat(r["expected"]), ZScore: asFloat(r["zscore"]),
	}, nil
}

func fetchContributors(ctx context.Context, conn *chClient, alertID, metric string) ([]Segment, error) {
	if alertID == "" || !looksLikeUUID(alertID) {
		return nil, nil
	}
	q := fmt.Sprintf(`
		SELECT
			dimension,
			dimension_value,
			current_value,
			baseline_value,
			delta,
			contribution
		FROM alert_dimension_contributors
		WHERE alert_id = toUUID(%s)
		ORDER BY abs(delta) DESC
		LIMIT 8
	`, quoteString(alertID))
	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]Segment, 0, len(rows))
	for _, r := range rows {
		base := asFloat(r["baseline_value"])
		cur := asFloat(r["current_value"])
		contrib := asFloat(r["contribution"])
		// Stored as fraction (0–1) or already percent — normalize to percent.
		if math.Abs(contrib) <= 1.5 {
			contrib *= 100
		}
		out = append(out, Segment{
			Dimension: asString(r["dimension"]), Value: asString(r["dimension_value"]),
			Metric: metric, DeltaPct: round1(pctChange(cur, base)), ContributionPct: round1(contrib),
		})
	}
	return out, nil
}

type observationRow struct {
	Order int
	Type  string
	Title string
	Detail string
	Impact float64
}

func fetchObservations(ctx context.Context, conn *chClient, alertID string) ([]observationRow, error) {
	if alertID == "" || !looksLikeUUID(alertID) {
		return nil, nil
	}
	q := fmt.Sprintf(`
		SELECT observation_order, observation_type, title, detail, impact
		FROM alert_observations
		WHERE alert_id = toUUID(%s)
		ORDER BY observation_order
	`, quoteString(alertID))
	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]observationRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, observationRow{
			Order: int(asFloat(r["observation_order"])),
			Type: asString(r["observation_type"]), Title: asString(r["title"]),
			Detail: asString(r["detail"]), Impact: asFloat(r["impact"]),
		})
	}
	return out, nil
}

func querySnapshotMetrics(ctx context.Context, conn *chClient, advertiserID string, start, end time.Time) (metricBag, error) {
	where := fmt.Sprintf("bucket >= %s AND bucket <= %s", quoteTime(start), quoteTime(end))
	if advertiserID != "" {
		where += " AND advertiser_id = " + quoteString(advertiserID)
	}
	q := fmt.Sprintf(`
		SELECT
			sum(requests) AS requests,
			sum(fills) AS fills,
			sum(impressions) AS impressions,
			sum(clicks) AS clicks,
			sum(revenue) AS revenue
		FROM metric_hourly_snapshot
		WHERE %s
	`, where)
	return scanMetricBag(ctx, conn, q)
}

func querySeasonalBaseline(ctx context.Context, conn *chClient, advertiserID string, windowStart, windowEnd time.Time, weeks int) (metricBag, error) {
	day := time.Date(windowStart.Year(), windowStart.Month(), windowStart.Day(), windowStart.Hour(), 0, 0, 0, time.UTC)
	dur := windowEnd.Sub(windowStart)
	var sum metricBag
	n := 0
	for i := 1; i <= weeks; i++ {
		ws := day.AddDate(0, 0, -7*i)
		we := ws.Add(dur)
		m, err := querySnapshotMetrics(ctx, conn, advertiserID, ws, we)
		if err != nil {
			return metricBag{}, err
		}
		if m.Requests == 0 && m.Revenue == 0 {
			continue
		}
		sum.Requests += m.Requests
		sum.Fills += m.Fills
		sum.Impressions += m.Impressions
		sum.Clicks += m.Clicks
		sum.Revenue += m.Revenue
		n++
	}
	if n == 0 {
		// Fall back to baseline_hourly expected for revenue when snapshot history is thin.
		if advertiserID != "" {
			q := fmt.Sprintf(`
				SELECT expected
				FROM baseline_hourly
				WHERE advertiser_id = %s AND metric = 'revenue' AND bucket = %s
				LIMIT 1
			`, quoteString(advertiserID), quoteTime(day))
			rows, err := conn.QueryMaps(ctx, q)
			if err == nil && len(rows) > 0 {
				return metricBag{Revenue: asFloat(rows[0]["expected"])}, nil
			}
		}
		return metricBag{}, fmt.Errorf("no seasonal baseline rows in metric_hourly_snapshot")
	}
	return scaleBag(sum, 1/float64(n)), nil
}

func rankSegmentsFromSnapshot(ctx context.Context, conn *chClient, advertiserID string, obsStart, obsEnd time.Time, culprit string) ([]Segment, error) {
	dims := []string{"ad_format", "country", "os_version", "category", "vertical", "region", "campaign_type", "publisher_tier"}
	metricExpr, weightExpr := snapshotMetricSQL(culprit)
	advFilter := ""
	if advertiserID != "" {
		advFilter = " AND advertiser_id = " + quoteString(advertiserID)
	}
	day := time.Date(obsStart.Year(), obsStart.Month(), obsStart.Day(), obsStart.Hour(), 0, 0, 0, time.UTC)
	dur := obsEnd.Sub(obsStart)
	parts := make([]string, 0, 4)
	for i := 1; i <= 4; i++ {
		ws := day.AddDate(0, 0, -7*i)
		we := ws.Add(dur)
		parts = append(parts, fmt.Sprintf("(bucket >= %s AND bucket <= %s)", quoteTime(ws), quoteTime(we)))
	}
	basePred := strings.Join(parts, " OR ")

	var all []Segment
	for _, dim := range dims {
		q := fmt.Sprintf(`
			WITH
			obs AS (
				SELECT %s AS dim, %s AS metric, %s AS weight
				FROM metric_hourly_snapshot
				WHERE bucket >= %s AND bucket <= %s%s
				GROUP BY dim
			),
			base AS (
				SELECT dim, avg(metric) AS metric, avg(weight) AS weight
				FROM (
					SELECT %s AS dim, %s AS metric, %s AS weight, toDate(bucket) AS d
					FROM metric_hourly_snapshot
					WHERE (%s)%s
					GROUP BY dim, d
				)
				GROUP BY dim
			)
			SELECT
				toString(obs.dim) AS dim,
				obs.metric AS obs_metric,
				base.metric AS base_metric,
				obs.weight AS obs_weight
			FROM obs
			INNER JOIN base ON obs.dim = base.dim
			WHERE obs.dim IS NOT NULL AND toString(obs.dim) != ''
			ORDER BY abs(obs.metric - base.metric) * abs(obs.weight) DESC
			LIMIT 3
		`, dim, metricExpr, weightExpr, quoteTime(obsStart), quoteTime(obsEnd), advFilter,
			dim, metricExpr, weightExpr, basePred, advFilter)

		rows, err := conn.QueryMaps(ctx, q)
		if err != nil {
			return nil, err
		}
		var totalAbs float64
		type raw struct {
			value            string
			obsM, baseM, wgt float64
		}
		raws := make([]raw, 0, len(rows))
		for _, row := range rows {
			r := raw{
				value: asString(row["dim"]), obsM: asFloat(row["obs_metric"]),
				baseM: asFloat(row["base_metric"]), wgt: asFloat(row["obs_weight"]),
			}
			totalAbs += math.Abs(r.obsM-r.baseM) * math.Max(math.Abs(r.wgt), 1)
			raws = append(raws, r)
		}
		for _, r := range raws {
			contrib := 0.0
			if totalAbs > 0 {
				contrib = math.Abs(r.obsM-r.baseM) * math.Max(math.Abs(r.wgt), 1) / totalAbs * 100
			}
			all = append(all, Segment{
				Dimension: dim, Value: r.value, Metric: culprit,
				DeltaPct: round1(pctChange(r.obsM, r.baseM)), ContributionPct: round1(contrib),
			})
		}
	}
	if len(all) > 8 {
		// Keep highest |contribution|
		for i := 0; i < len(all); i++ {
			for j := i + 1; j < len(all); j++ {
				if math.Abs(all[j].ContributionPct) > math.Abs(all[i].ContributionPct) {
					all[i], all[j] = all[j], all[i]
				}
			}
		}
		all = all[:8]
	}
	return all, nil
}

func snapshotMetricSQL(culprit string) (string, string) {
	switch culprit {
	case "requests":
		return "sum(requests)", "sum(requests)"
	case "fill_rate":
		return "sum(fills) / nullIf(sum(requests),0)", "sum(fills)"
	case "render_rate":
		return "sum(impressions) / nullIf(sum(fills),0)", "sum(impressions)"
	case "ecpm":
		return "sum(revenue) / nullIf(sum(impressions),0) * 1000", "sum(revenue)"
	case "ctr":
		return "sum(clicks) / nullIf(sum(impressions),0)", "sum(clicks)"
	default:
		return "sum(revenue)", "sum(revenue)"
	}
}

func scanMetricBag(ctx context.Context, conn *chClient, q string) (metricBag, error) {
	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return metricBag{}, err
	}
	if len(rows) == 0 {
		return metricBag{}, nil
	}
	r := rows[0]
	return metricBag{
		Requests: asFloat(r["requests"]), Fills: asFloat(r["fills"]),
		Impressions: asFloat(r["impressions"]), Clicks: asFloat(r["clicks"]),
		Revenue: asFloat(r["revenue"]),
	}, nil
}

func resolveWindow(startStr, endStr string) (time.Time, time.Time, error) {
	if startStr == "" || endStr == "" {
		start, _ := time.Parse(time.RFC3339, "2026-06-21T16:00:00Z")
		end := start.Add(time.Hour - time.Second)
		return start, end, nil
	}
	start, err := parseFlexibleTime(startStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := parseFlexibleTime(endStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start.UTC(), end.UTC(), nil
}

func parseFlexibleTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid time: %s", s)
}

func parseCHTime(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid clickhouse time: %s", s)
}

func scaleBag(m metricBag, scale float64) metricBag {
	return metricBag{
		Requests: m.Requests * scale, Fills: m.Fills * scale,
		Impressions: m.Impressions * scale, Clicks: m.Clicks * scale,
		Revenue: m.Revenue * scale,
	}
}

func decompose(base, obs metricBag) []Factor {
	mk := func(factor, label string, b, o float64) Factor {
		return Factor{Factor: factor, Label: label, Baseline: b, Observed: o, DeltaPct: round1(pctChange(o, b)), Status: "neutral"}
	}
	return []Factor{
		mk("requests", "Requests", base.Requests, obs.Requests),
		mk("fill_rate", "Fill rate", base.FillRate(), obs.FillRate()),
		mk("render_rate", "Render rate", base.RenderRate(), obs.RenderRate()),
		mk("ecpm", "eCPM", base.ECPM(), obs.ECPM()),
		mk("ctr", "CTR", base.CTR(), obs.CTR()),
	}
}

func pickCulprit(factors []Factor, alertMetric string) string {
	if alertMetric == "ctr" || alertMetric == "fill_rate" || alertMetric == "ecpm" || alertMetric == "requests" {
		for i := range factors {
			if factors[i].Factor == alertMetric {
				factors[i].Status = "culprit"
			} else if math.Abs(factors[i].DeltaPct) < 3 {
				factors[i].Status = "ruled_out"
			}
		}
		return alertMetric
	}
	best := "fill_rate"
	bestAbs := -1.0
	for _, f := range factors {
		if f.Factor == "ctr" {
			continue
		}
		if math.Abs(f.DeltaPct) > bestAbs {
			bestAbs = math.Abs(f.DeltaPct)
			best = f.Factor
		}
	}
	for i := range factors {
		if factors[i].Factor == best {
			factors[i].Status = "culprit"
		} else if math.Abs(factors[i].DeltaPct) < 3 {
			factors[i].Status = "ruled_out"
		} else {
			factors[i].Status = "neutral"
		}
	}
	return best
}

func buildRuledOut(culprit string, base, obs metricBag, revenuePct float64) []RuledOut {
	out := []RuledOut{}
	if math.Abs(pctChange(obs.Requests, base.Requests)) < 3 {
		out = append(out, RuledOut{
			Reason: "Request volume",
			Detail: fmt.Sprintf("Requests changed only %.1f%% vs baseline.", pctChange(obs.Requests, base.Requests)),
		})
	}
	if math.Abs(pctChange(obs.ECPM(), base.ECPM())) < 3 && culprit != "ecpm" {
		out = append(out, RuledOut{
			Reason: "eCPM / price",
			Detail: fmt.Sprintf("eCPM moved %.1f%%, too small to explain the move.", pctChange(obs.ECPM(), base.ECPM())),
		})
	}
	if culprit != "ctr" {
		out = append(out, RuledOut{
			Reason: "CTR",
			Detail: fmt.Sprintf("CTR %.1f%%; not a direct revenue factor in the CPM identity.", pctChange(obs.CTR(), base.CTR())),
		})
	}
	if math.Abs(revenuePct) >= 5 {
		out = append(out, RuledOut{
			Reason: "Seasonality check",
			Detail: "Same hour-of-week / 4-week InsightIQ baseline applied; residual movement remains.",
		})
	}
	return out
}

func buildDiagnosisFromInsightIQ(
	alert Alert,
	decomp []Factor,
	culprit string,
	segments []Segment,
	ruled []RuledOut,
	observations []observationRow,
	live *alertLiveRow,
) Diagnosis {
	citations := []Citation{{Label: humanMetric(alert.Metric) + " change", Value: formatSignedPct(alert.PctChange)}}
	if live != nil {
		citations = append(citations,
			Citation{Label: "Actual", Value: formatNumber(live.Actual)},
			Citation{Label: "Expected", Value: formatNumber(live.Expected)},
			Citation{Label: "Z-score", Value: fmt.Sprintf("%.2f", live.ZScore)},
		)
	}

	if len(observations) > 0 {
		parts := make([]string, 0, len(observations))
		for _, o := range observations {
			parts = append(parts, o.Detail)
			citations = append(citations, Citation{Label: o.Title, Value: o.Detail})
		}
		text := strings.Join(parts, " ")
		if alert.AdvertiserID != "" {
			text = fmt.Sprintf("%s for %s %s %s. %s",
				humanMetric(alert.Metric), alert.AdvertiserID, alert.Direction, formatPct(math.Abs(alert.PctChange)), text)
		}
		return Diagnosis{Text: text, Citations: citations}
	}

	var culpritFactor *Factor
	for i := range decomp {
		if decomp[i].Factor == culprit {
			culpritFactor = &decomp[i]
			break
		}
	}
	top := ""
	if len(segments) > 0 {
		parts := []string{}
		for i, s := range segments {
			if i >= 3 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s %s (%s)",
				humanDimension(s.Dimension), s.Value, formatSignedPct(s.DeltaPct)))
		}
		top = strings.Join(parts, "; ")
	}

	text := fmt.Sprintf("%s %s %s, primarily driven by %s",
		humanMetric(alert.Metric), alert.Direction, formatPct(math.Abs(alert.PctChange)), humanMetric(culprit))
	if culpritFactor != nil {
		text += fmt.Sprintf(" (%s to %s, %s)",
			formatFactorValue(culprit, culpritFactor.Baseline),
			formatFactorValue(culprit, culpritFactor.Observed),
			formatSignedPct(culpritFactor.DeltaPct),
		)
		citations = append(citations, Citation{
			Label: culpritFactor.Label + " change",
			Value: formatSignedPct(culpritFactor.DeltaPct),
		})
	}
	if top != "" {
		text += ". Top segments: " + top + "."
	}
	if len(ruled) > 0 {
		text += " Ruled out: " + ruled[0].Reason + "."
	}
	for i, s := range segments {
		if i >= 3 {
			break
		}
		citations = append(citations, Citation{
			Label: fmt.Sprintf("%s: %s", humanDimension(s.Dimension), s.Value),
			Value: fmt.Sprintf("%s (contribution %s)", formatSignedPct(s.DeltaPct), formatPct(s.ContributionPct)),
		})
	}
	return Diagnosis{Text: text, Citations: citations}
}

func humanMetric(m string) string {
	switch m {
	case "fill_rate":
		return "Fill rate"
	case "render_rate":
		return "Render rate"
	case "ecpm":
		return "eCPM"
	case "ctr":
		return "CTR"
	case "rpr":
		return "Revenue per request"
	case "requests":
		return "Requests"
	case "impressions":
		return "Impressions"
	case "revenue":
		return "Revenue"
	default:
		s := strings.ReplaceAll(m, "_", " ")
		if s == "" {
			return s
		}
		return strings.ToUpper(s[:1]) + s[1:]
	}
}

func humanDimension(d string) string {
	switch d {
	case "ad_format":
		return "Ad format"
	case "publisher_tier":
		return "Publisher tier"
	case "campaign_type":
		return "Campaign type"
	case "device_model":
		return "Device"
	case "os_version":
		return "OS"
	default:
		return humanMetric(d)
	}
}

func formatPct(v float64) string {
	return fmt.Sprintf("%.1f%%", v)
}

func formatSignedPct(v float64) string {
	if v > 0 {
		return fmt.Sprintf("+%.1f%%", v)
	}
	return fmt.Sprintf("%.1f%%", v)
}

func formatNumber(v float64) string {
	if math.Abs(v) >= 1000 {
		return formatInt(v)
	}
	return fmt.Sprintf("%.4f", v)
}

func formatFactorValue(factor string, v float64) string {
	switch factor {
	case "fill_rate", "render_rate", "ctr":
		return fmt.Sprintf("%.1f%%", v*100)
	case "ecpm":
		return fmt.Sprintf("$%.2f", v)
	case "requests", "impressions", "clicks", "fills":
		return formatInt(v)
	default:
		if math.Abs(v) >= 1000 {
			return formatInt(v)
		}
		return fmt.Sprintf("%.2f", v)
	}
}

func formatInt(v float64) string {
	n := int64(math.Round(v))
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func metricValue(m metricBag, metric string) float64 {
	switch metric {
	case "fill_rate":
		return m.FillRate()
	case "requests":
		return m.Requests
	case "impressions":
		return m.Impressions
	case "ctr":
		return m.CTR()
	case "ecpm":
		return m.ECPM()
	case "rpr":
		return m.RPR()
	default:
		return m.Revenue
	}
}

func pctChange(obs, base float64) float64 {
	if base == 0 {
		if obs == 0 {
			return 0
		}
		return 100
	}
	return (obs - base) / math.Abs(base) * 100
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func severityFromZOrPct(live *alertLiveRow, absPct float64) string {
	if live != nil {
		z := math.Abs(live.ZScore)
		switch {
		case z >= 10:
			return "critical"
		case z >= 5:
			return "high"
		case z >= 3:
			return "medium"
		default:
			return "low"
		}
	}
	return severityFrom(absPct)
}

func severityFrom(absPct float64) string {
	switch {
	case absPct >= 12:
		return "critical"
	case absPct >= 8:
		return "high"
	case absPct >= 4:
		return "medium"
	default:
		return "low"
	}
}

func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

func requestFromInvestigationID(id string) (InvestigateRequest, error) {
	id = normalizeAlertID(id)
	if looksLikeUUID(id) {
		return InvestigateRequest{AlertID: id}, nil
	}
	// Legacy: inv-{metric}-{YYYYMMDD}
	parts := strings.Split(id, "-")
	if len(parts) < 2 {
		return InvestigateRequest{}, fmt.Errorf("invalid investigation id")
	}
	datePart := parts[len(parts)-1]
	metric := strings.Join(parts[:len(parts)-1], "_")
	if metric == "" {
		metric = "revenue"
	}
	day, err := time.Parse("20060102", datePart)
	if err != nil {
		return InvestigateRequest{}, err
	}
	ws := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	we := ws.Add(24*time.Hour - time.Second)
	return InvestigateRequest{
		AlertID:      fmt.Sprintf("alert-%s-%s", metric, day.Format("2006-01-02")),
		Metric:       metric,
		WindowStart:  ws.Format(time.RFC3339),
		WindowEnd:    we.Format(time.RFC3339),
		BaselineKind: "same_hour_4w_seasonality",
	}, nil
}

func detectAlerts(ctx context.Context, conn *chClient, cache *invCache) ([]map[string]any, error) {
	// Prefer alerts that already have ClickHouse-native observations (demo-ready RCA).
	q := `
		SELECT
			toString(a.alert_id) AS id,
			a.advertiser_id,
			a.metric,
			toString(a.bucket) AS bucket,
			a.actual,
			a.expected,
			a.zscore,
			count(o.alert_id) AS obs_n
		FROM alerts_live AS a
		LEFT JOIN alert_observations AS o ON o.alert_id = a.alert_id
		WHERE abs(a.zscore) > 3
		GROUP BY a.alert_id, a.advertiser_id, a.metric, a.bucket, a.actual, a.expected, a.zscore
		ORDER BY obs_n DESC, abs(a.zscore) DESC
		LIMIT 12
	`
	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		alertID := asString(r["id"])
		inv, err := runInvestigation(ctx, conn, InvestigateRequest{AlertID: alertID})
		if err != nil {
			continue
		}
		if cache != nil {
			cache.put(inv)
		}
		out = append(out, map[string]any{
			"id": inv.Alert.ID, "metric": inv.Alert.Metric, "direction": inv.Alert.Direction,
			"pctChange": inv.Alert.PctChange, "windowStart": inv.Alert.WindowStart, "windowEnd": inv.Alert.WindowEnd,
			"baselineKind": inv.Alert.BaselineKind, "severity": inv.Alert.Severity,
			"advertiserId": inv.Alert.AdvertiserID,
			"investigationId": inv.ID, "status": inv.Status, "summary": inv.Diagnosis.Text,
		})
	}
	return out, nil
}
