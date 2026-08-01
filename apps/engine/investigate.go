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
	ID             string           `json:"id"`
	Status         string           `json:"status"`
	Alert          Alert            `json:"alert"`
	Decomposition  []Factor         `json:"decomposition"`
	Segments       []Segment        `json:"segments"`
	RuledOut       []RuledOut       `json:"ruledOut"`
	Diagnosis      Diagnosis        `json:"diagnosis"`
	Trace          []TraceStep      `json:"trace"`
	Seasonality    SeasonalityCheck `json:"seasonality"`
	Waterfall      []WaterfallStep  `json:"waterfall"`
	Counterfactual Counterfactual   `json:"counterfactual"`
	Hypotheses     []Hypothesis     `json:"hypotheses"`
	Evidence       EvidenceLock     `json:"evidence"`
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
	// Only look up alerts_live for real UUIDs. Legacy demo ids (alert-rev-YYYY-MM-DD)
	// use the window fields below instead.
	if alertID != "" && looksLikeUUID(alertID) {
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
	flatStart := windowStart.Add(-24 * time.Hour)
	flatEnd := windowEnd.Add(-24 * time.Hour)
	flat, err := querySnapshotMetrics(ctx, conn, advertiserID, flatStart, flatEnd)
	if err != nil {
		flat = metricBag{}
	}
	mark("baseline", fmt.Sprintf("metric_hourly_snapshot vs %s (+ flat prior-day check)", baselineKind), t1)

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
		// Enrich with wall categories (region, campaign_type, publisher_tier, …)
		// when ClickHouse contributors only cover a subset of dimensions.
		extra, err := rankSegmentsFromSnapshot(ctx, conn, advertiserID, windowStart, windowEnd, culprit)
		if err == nil {
			segments = mergeSegments(segments, extra, 8)
		}
		mark("slice", fmt.Sprintf("Loaded contributors + snapshot dims (%d)", len(segments)), t3)
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

	tSeas := time.Now()
	seasonality := evaluateSeasonality(metric, observed, baseline, flat, pct)
	mark("seasonality", seasonality.Detail, tSeas)

	ruledOut := buildRuledOut(culprit, baseline, observed, pct)
	ruledOut = enrichRuledOutWithSeasonality(ruledOut, seasonality, culprit, baseline, observed, pct)
	waterfall := buildWaterfall(baseline, observed, decomp)
	counterfactual := buildCounterfactual(culprit, baseline, observed)
	hypotheses := buildHypotheses(decomp, culprit, segments)
	diagnosis := buildDiagnosisFromInsightIQ(alert, decomp, culprit, segments, ruledOut, observations, live)
	diagnosis = appendCounterfactualCitation(diagnosis, counterfactual)
	mark("evidence", "Packaged evidence + waterfall + counterfactual + hypotheses", t4)
	trace = append(trace, TraceStep{Step: "narrate", Detail: "Narration deferred to Node/Gemini from evidence only", DurationMs: 0})

	uiSegments := segments
	if len(uiSegments) > 6 {
		uiSegments = uiSegments[:6]
	}

	inv := &Investigation{
		ID: invID, Status: "complete", Alert: alert,
		Decomposition: decomp, Segments: uiSegments, RuledOut: ruledOut,
		Diagnosis: diagnosis, Trace: trace,
		Seasonality: seasonality, Waterfall: waterfall,
		Counterfactual: counterfactual, Hypotheses: hypotheses,
	}
	if seasonality.Status == "ruled_out_as_seasonality" {
		inv.Alert.Severity = "low"
		inv.Diagnosis.Text = "Seasonality trap: " + seasonality.Detail + " " + inv.Diagnosis.Text
	}
	inv.Evidence = buildEvidenceLock(inv)
	return inv, nil
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
		LIMIT 8
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

	var culpritFactor *Factor
	for i := range decomp {
		if decomp[i].Factor == culprit {
			culpritFactor = &decomp[i]
			break
		}
	}

	// Keep diagnosis short for the UI: headline + top drivers only (not every observation row).
	text := fmt.Sprintf("%s %s %s",
		humanMetric(alert.Metric), alert.Direction, formatPct(math.Abs(alert.PctChange)))
	if alert.AdvertiserID != "" {
		text = fmt.Sprintf("%s for %s %s %s",
			humanMetric(alert.Metric), alert.AdvertiserID, alert.Direction, formatPct(math.Abs(alert.PctChange)))
	}
	if culprit != "" {
		text += ", primarily driven by " + humanMetric(culprit)
		if culpritFactor != nil {
			text += fmt.Sprintf(" (%s → %s, %s)",
				formatFactorValue(culprit, culpritFactor.Baseline),
				formatFactorValue(culprit, culpritFactor.Observed),
				formatSignedPct(culpritFactor.DeltaPct),
			)
			citations = append(citations, Citation{
				Label: culpritFactor.Label + " change",
				Value: formatSignedPct(culpritFactor.DeltaPct),
			})
		}
	}

	topSegs := segments
	if len(topSegs) > 3 {
		topSegs = topSegs[:3]
	}
	if len(topSegs) > 0 {
		parts := make([]string, 0, len(topSegs))
		for _, s := range topSegs {
			parts = append(parts, fmt.Sprintf("%s=%s (%s, contrib %s)",
				humanDimension(s.Dimension), s.Value, formatSignedPct(s.DeltaPct), formatPct(s.ContributionPct)))
			citations = append(citations, Citation{
				Label: fmt.Sprintf("%s: %s", humanDimension(s.Dimension), s.Value),
				Value: fmt.Sprintf("%s · contrib %s", formatSignedPct(s.DeltaPct), formatPct(s.ContributionPct)),
			})
		}
		text += ". Top segments: " + strings.Join(parts, "; ") + "."
	} else if len(observations) > 0 {
		// Fallback when snapshot segments are empty but CH observations exist.
		limit := 3
		if len(observations) < limit {
			limit = len(observations)
		}
		parts := make([]string, 0, limit)
		for i := 0; i < limit; i++ {
			o := observations[i]
			parts = append(parts, shortenObservationDetail(o.Detail))
			citations = append(citations, Citation{
				Label: fmt.Sprintf("%s (%d)", o.Title, i+1),
				Value: shortenObservationDetail(o.Detail),
			})
		}
		text += ". " + strings.Join(parts, " ")
	}

	if len(ruled) > 0 {
		text += " Ruled out: " + ruled[0].Reason + "."
	}
	return Diagnosis{Text: text, Citations: citations}
}

func shortenObservationDetail(detail string) string {
	d := strings.TrimSpace(detail)
	// Strip verbose "Dimension value \"x\" went from..." boilerplate when possible.
	if strings.Contains(d, `Dimension value "`) {
		re := strings.NewReplacer(`Dimension value "`, "", `" went from`, ":", " (delta:", " Δ", ", contribution:", " ·")
		d = re.Replace(d)
	}
	if len(d) > 140 {
		return d[:137] + "…"
	}
	return d
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

// Alert wall categories exposed to the UI.
var alertCategoryOrder = []string{"geo", "os", "campaign_type", "ad_format", "publisher_tier", "content"}

func categoryForDimension(dim string) string {
	switch strings.ToLower(dim) {
	case "country", "region":
		return "geo"
	case "os_version", "os":
		return "os"
	case "campaign_type":
		return "campaign_type"
	case "ad_format":
		return "ad_format"
	case "publisher_tier":
		return "publisher_tier"
	case "category", "vertical":
		return "content"
	default:
		return ""
	}
}

func categorizeSegments(segments []Segment) (categories []string, primary string, labels []map[string]any) {
	seen := map[string]bool{}
	bestByCat := map[string]Segment{}
	for _, s := range segments {
		cat := categoryForDimension(s.Dimension)
		if cat == "" {
			continue
		}
		seen[cat] = true
		prev, ok := bestByCat[cat]
		if !ok || math.Abs(s.ContributionPct) > math.Abs(prev.ContributionPct) {
			bestByCat[cat] = s
		}
	}
	ordered := make([]string, 0, len(seen))
	for _, c := range alertCategoryOrder {
		if seen[c] {
			ordered = append(ordered, c)
		}
	}
	if len(ordered) > 0 {
		primary = ordered[0]
	}
	for _, c := range ordered {
		s := bestByCat[c]
		labels = append(labels, map[string]any{
			"category": c,
			"dimension": s.Dimension,
			"value": s.Value,
			"deltaPct": s.DeltaPct,
			"contributionPct": s.ContributionPct,
		})
	}
	return ordered, primary, labels
}

func mergeSegments(primary, extra []Segment, limit int) []Segment {
	seen := map[string]bool{}
	out := make([]Segment, 0, limit)
	key := func(s Segment) string { return s.Dimension + "|" + s.Value }
	for _, s := range primary {
		k := key(s)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	for _, s := range extra {
		if categoryForDimension(s.Dimension) == "" {
			continue
		}
		k := key(s)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

type alertListRow struct {
	AlertID      string
	AdvertiserID string
	Metric       string
	Bucket       time.Time
	Actual       float64
	Expected     float64
	ZScore       float64
}

// detectAlerts builds the alert wall from alerts_live + contributors.
// granularity: "day" (default) collapses to the peak |z| alert per advertiser+metric+UTC day;
// "hour" keeps native hourly buckets from alerts_live.
// Full investigations still run on open (peak-hour alert id is used for day cards).
func detectAlerts(ctx context.Context, conn *chClient, _ *invCache, granularity string) ([]map[string]any, error) {
	if granularity != "hour" {
		granularity = "day"
	}
	rows, err := selectAlertListRows(ctx, conn, 28, granularity)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []map[string]any{}, nil
	}

	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.AlertID)
	}
	segsByAlert, err := fetchContributorsBatch(ctx, conn, ids, 8)
	if err != nil {
		return nil, err
	}
	obsByAlert, err := fetchObservationDetailsBatch(ctx, conn, ids, 3)
	if err != nil {
		// Observations are optional enrichment; don't fail the wall.
		obsByAlert = map[string][]string{}
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		pct := 0.0
		if r.Expected != 0 {
			pct = (r.Actual - r.Expected) / math.Abs(r.Expected) * 100
		}
		direction := "down"
		if pct > 0 {
			direction = "up"
		}
		metric := r.Metric
		if metric == "" {
			metric = "revenue"
		}
		segments := segsByAlert[r.AlertID]
		for i := range segments {
			if segments[i].Metric == "" {
				segments[i].Metric = metric
			}
		}
		categories, primary, labels := categorizeSegments(segments)
		live := &alertLiveRow{
			AlertID: r.AlertID, AdvertiserID: r.AdvertiserID, Metric: metric,
			Bucket: r.Bucket, Actual: r.Actual, Expected: r.Expected, ZScore: r.ZScore,
		}
		var windowStart, windowEnd time.Time
		baselineKind := "same_hour_4w_seasonality"
		if granularity == "day" {
			day := time.Date(r.Bucket.UTC().Year(), r.Bucket.UTC().Month(), r.Bucket.UTC().Day(), 0, 0, 0, 0, time.UTC)
			windowStart = day
			windowEnd = day.Add(24*time.Hour - time.Second)
			baselineKind = "daily_peak_hour"
		} else {
			windowStart = r.Bucket.UTC()
			windowEnd = r.Bucket.Add(time.Hour - time.Second).UTC()
		}
		summary := buildAlertListSummary(r.AdvertiserID, metric, direction, pct, segments, obsByAlert[r.AlertID])
		item := map[string]any{
			"id": r.AlertID, "metric": metric, "direction": direction,
			"pctChange":       round1(pct),
			"windowStart":    windowStart.Format(time.RFC3339),
			"windowEnd":      windowEnd.Format(time.RFC3339),
			"baselineKind":   baselineKind,
			"granularity":    granularity,
			"sourceBucket":   r.Bucket.UTC().Format(time.RFC3339),
			"severity":      severityFromZOrPct(live, math.Abs(pct)),
			"advertiserId":   r.AdvertiserID,
			"investigationId": "inv-" + r.AlertID,
			"status":          "complete",
			"summary":         summary,
			"categories":      categories,
			"primaryCategory": primary,
			"categoryLabels":  labels,
		}
		out = append(out, item)
	}
	return out, nil
}

func buildAlertListSummary(advertiserID, metric, direction string, pct float64, segments []Segment, obs []string) string {
	head := fmt.Sprintf("%s %s %s", humanMetric(metric), direction, formatPct(math.Abs(pct)))
	if advertiserID != "" {
		head = fmt.Sprintf("%s for %s %s %s", humanMetric(metric), advertiserID, direction, formatPct(math.Abs(pct)))
	}
	if len(obs) > 0 {
		return head + ". " + strings.Join(obs, " ")
	}
	if len(segments) == 0 {
		return head + "."
	}
	parts := make([]string, 0, 3)
	for i, s := range segments {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %s (%s)",
			humanDimension(s.Dimension), s.Value, formatSignedPct(s.DeltaPct)))
	}
	return head + ". Top segments: " + strings.Join(parts, "; ") + "."
}

func selectAlertListRows(ctx context.Context, conn *chClient, limit int, granularity string) ([]alertListRow, error) {
	if limit <= 0 {
		limit = 24
	}
	if granularity != "hour" {
		granularity = "day"
	}
	seen := map[string]bool{}
	out := make([]alertListRow, 0, limit)

	appendRows := func(rows []map[string]any) {
		for _, r := range rows {
			id := asString(r["id"])
			if id == "" || seen[id] {
				continue
			}
			bucket, err := parseCHTime(asString(r["bucket"]))
			if err != nil {
				continue
			}
			seen[id] = true
			out = append(out, alertListRow{
				AlertID: id, AdvertiserID: asString(r["advertiser_id"]),
				Metric: asString(r["metric"]), Bucket: bucket,
				Actual: asFloat(r["actual"]), Expected: asFloat(r["expected"]), ZScore: asFloat(r["zscore"]),
			})
			if len(out) >= limit {
				return
			}
		}
	}

	// Peak |z| per advertiser+metric+UTC day (alerts_live is hourly-only today).
	const dailyPeaksCTE = `
		WITH daily_peaks AS (
			SELECT
				alert_id,
				advertiser_id,
				metric,
				bucket,
				actual,
				expected,
				zscore
			FROM (
				SELECT
					alert_id,
					advertiser_id,
					metric,
					bucket,
					actual,
					expected,
					zscore,
					row_number() OVER (
						PARTITION BY advertiser_id, metric, toDate(bucket)
						ORDER BY abs(zscore) DESC
					) AS day_rn
				FROM alerts_live
				WHERE abs(zscore) > 3
			)
			WHERE day_rn = 1
		)
	`

	richLimit := limit / 3
	if richLimit < 6 {
		richLimit = 6
	}
	if richLimit > limit {
		richLimit = limit
	}

	var rich []map[string]any
	var err error
	if granularity == "day" {
		rich, err = conn.QueryMaps(ctx, fmt.Sprintf(`
			%s
			SELECT
				toString(alert_id) AS id,
				advertiser_id,
				metric,
				toString(bucket) AS bucket,
				actual,
				expected,
				zscore
			FROM daily_peaks
			WHERE alert_id IN (SELECT DISTINCT alert_id FROM alert_dimension_contributors)
			   OR alert_id IN (SELECT DISTINCT alert_id FROM alert_observations)
			ORDER BY abs(zscore) DESC
			LIMIT %d
		`, dailyPeaksCTE, richLimit))
	} else {
		rich, err = conn.QueryMaps(ctx, fmt.Sprintf(`
			SELECT
				toString(a.alert_id) AS id,
				a.advertiser_id AS advertiser_id,
				a.metric AS metric,
				toString(a.bucket) AS bucket,
				a.actual AS actual,
				a.expected AS expected,
				a.zscore AS zscore
			FROM alerts_live AS a
			WHERE abs(a.zscore) > 3
			  AND (
				a.alert_id IN (SELECT DISTINCT alert_id FROM alert_dimension_contributors)
				OR a.alert_id IN (SELECT DISTINCT alert_id FROM alert_observations)
			  )
			ORDER BY abs(a.zscore) DESC
			LIMIT %d
		`, richLimit))
	}
	if err != nil {
		return nil, err
	}
	appendRows(rich)
	if len(out) >= limit {
		return out, nil
	}

	excludeIDs := make([]string, 0, len(out))
	for _, r := range out {
		excludeIDs = append(excludeIDs, "toUUID("+quoteString(r.AlertID)+")")
	}
	excludeIDClause := ""
	if len(excludeIDs) > 0 {
		excludeIDClause = " AND alert_id NOT IN (" + strings.Join(excludeIDs, ",") + ")"
	}

	excludeAds := make([]string, 0, len(out))
	for _, r := range out {
		if r.AdvertiserID != "" {
			excludeAds = append(excludeAds, quoteString(r.AdvertiserID))
		}
	}
	excludeAdClause := ""
	if len(excludeAds) > 0 {
		excludeAdClause = " AND advertiser_id NOT IN (" + strings.Join(excludeAds, ",") + ")"
	}

	var diverse []map[string]any
	if granularity == "day" {
		// One card per advertiser+day from remaining daily peaks.
		diverse, err = conn.QueryMaps(ctx, fmt.Sprintf(`
			%s
			SELECT
				toString(alert_id) AS id,
				advertiser_id,
				metric,
				toString(bucket) AS bucket,
				actual,
				expected,
				zscore
			FROM (
				SELECT
					alert_id,
					advertiser_id,
					metric,
					bucket,
					actual,
					expected,
					zscore,
					row_number() OVER (
						PARTITION BY advertiser_id, toDate(bucket)
						ORDER BY abs(zscore) DESC
					) AS rn
				FROM daily_peaks
				WHERE 1=1%s
			)
			WHERE rn = 1
			ORDER BY abs(zscore) DESC
			LIMIT %d
		`, dailyPeaksCTE, excludeIDClause, limit-len(out)))
	} else {
		diverse, err = conn.QueryMaps(ctx, fmt.Sprintf(`
			SELECT
				toString(alert_id) AS id,
				advertiser_id,
				metric,
				toString(bucket) AS bucket,
				actual,
				expected,
				zscore
			FROM (
				SELECT
					alert_id,
					advertiser_id,
					metric,
					bucket,
					actual,
					expected,
					zscore,
					row_number() OVER (PARTITION BY advertiser_id ORDER BY abs(zscore) DESC) AS rn
				FROM alerts_live
				WHERE abs(zscore) > 3%s
			)
			WHERE rn = 1
			ORDER BY abs(zscore) DESC
			LIMIT %d
		`, excludeAdClause, limit-len(out)))
	}
	if err != nil {
		return nil, err
	}
	appendRows(diverse)
	return out, nil
}

func fetchContributorsBatch(ctx context.Context, conn *chClient, alertIDs []string, perAlert int) (map[string][]Segment, error) {
	out := map[string][]Segment{}
	if len(alertIDs) == 0 {
		return out, nil
	}
	if perAlert <= 0 {
		perAlert = 8
	}
	quoted := make([]string, 0, len(alertIDs))
	for _, id := range alertIDs {
		if !looksLikeUUID(id) {
			continue
		}
		quoted = append(quoted, "toUUID("+quoteString(id)+")")
	}
	if len(quoted) == 0 {
		return out, nil
	}
	q := fmt.Sprintf(`
		SELECT
			toString(alert_id) AS alert_id,
			dimension,
			dimension_value,
			current_value,
			baseline_value,
			delta,
			contribution
		FROM (
			SELECT
				alert_id,
				dimension,
				dimension_value,
				current_value,
				baseline_value,
				delta,
				contribution,
				row_number() OVER (PARTITION BY alert_id ORDER BY abs(delta) DESC) AS rn
			FROM alert_dimension_contributors
			WHERE alert_id IN (%s)
		)
		WHERE rn <= %d
		ORDER BY alert_id, abs(delta) DESC
	`, strings.Join(quoted, ","), perAlert)
	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		id := asString(r["alert_id"])
		base := asFloat(r["baseline_value"])
		cur := asFloat(r["current_value"])
		contrib := asFloat(r["contribution"])
		if math.Abs(contrib) <= 1.5 {
			contrib *= 100
		}
		out[id] = append(out[id], Segment{
			Dimension: asString(r["dimension"]), Value: asString(r["dimension_value"]),
			DeltaPct: round1(pctChange(cur, base)), ContributionPct: round1(contrib),
		})
	}
	return out, nil
}

func fetchObservationDetailsBatch(ctx context.Context, conn *chClient, alertIDs []string, perAlert int) (map[string][]string, error) {
	out := map[string][]string{}
	if len(alertIDs) == 0 {
		return out, nil
	}
	if perAlert <= 0 {
		perAlert = 3
	}
	quoted := make([]string, 0, len(alertIDs))
	for _, id := range alertIDs {
		if !looksLikeUUID(id) {
			continue
		}
		quoted = append(quoted, "toUUID("+quoteString(id)+")")
	}
	if len(quoted) == 0 {
		return out, nil
	}
	q := fmt.Sprintf(`
		SELECT
			toString(alert_id) AS alert_id,
			detail
		FROM (
			SELECT
				alert_id,
				detail,
				row_number() OVER (PARTITION BY alert_id ORDER BY observation_order ASC) AS rn
			FROM alert_observations
			WHERE alert_id IN (%s)
		)
		WHERE rn <= %d
		ORDER BY alert_id, rn
	`, strings.Join(quoted, ","), perAlert)
	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		id := asString(r["alert_id"])
		detail := strings.TrimSpace(asString(r["detail"]))
		if detail == "" {
			continue
		}
		out[id] = append(out[id], detail)
	}
	return out, nil
}
