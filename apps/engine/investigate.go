package main

import (
	"context"
	"fmt"
	"math"
	"sort"
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
}

func runInvestigation(ctx context.Context, conn *chClient, req InvestigateRequest) (*Investigation, error) {
	trace := []TraceStep{}
	mark := func(step, detail string, started time.Time) {
		trace = append(trace, TraceStep{Step: step, Detail: detail, DurationMs: time.Since(started).Milliseconds()})
	}

	t0 := time.Now()
	windowStart, windowEnd, err := resolveWindow(req.WindowStart, req.WindowEnd)
	if err != nil {
		return nil, err
	}
	baselineKind := req.BaselineKind
	if baselineKind == "" {
		baselineKind = "same_weekday_trailing"
	}
	metric := req.Metric
	if metric == "" {
		metric = "revenue"
	}

	observed, err := queryMetrics(ctx, conn, windowStart, windowEnd)
	if err != nil {
		return nil, fmt.Errorf("observed metrics: %w", err)
	}

	var baseline metricBag
	if baselineKind == "same_weekday_trailing" {
		baseline, err = querySameWeekdayBaseline(ctx, conn, windowStart, windowEnd, 3)
	} else {
		baselineStart, baselineEnd := baselineRange(windowStart, windowEnd, baselineKind)
		baseline, err = queryMetrics(ctx, conn, baselineStart, baselineEnd)
		if err == nil {
			baselineDays := math.Max(1, baselineEnd.Sub(baselineStart).Hours()/24)
			observedDays := math.Max(1, windowEnd.Sub(windowStart).Hours()/24)
			baseline = scaleBag(baseline, observedDays/baselineDays)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("baseline metrics: %w", err)
	}
	mark("baseline", fmt.Sprintf("Compared %s..%s vs %s baseline",
		windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339), baselineKind), t0)

	t1 := time.Now()
	decomp := decompose(baseline, observed)
	culprit := pickCulprit(decomp, metric)
	mark("decompose", fmt.Sprintf("Revenue identity walk; culprit=%s", culprit), t1)

	t2 := time.Now()
	segments, err := rankSegments(ctx, conn, windowStart, windowEnd, baselineKind, culprit)
	if err != nil {
		return nil, fmt.Errorf("segments: %w", err)
	}
	mark("slice", fmt.Sprintf("Ranked dimensions for %s (%d segments)", culprit, len(segments)), t2)

	t3 := time.Now()
	pct := pctChange(metricValue(observed, metric), metricValue(baseline, metric))
	direction := "down"
	if pct > 0 {
		direction = "up"
	}
	alertID := req.AlertID
	if alertID == "" {
		alertID = fmt.Sprintf("alert-%s-%s", metric, windowStart.Format("2006-01-02"))
	}
	invID := fmt.Sprintf("inv-%s-%s", metric, windowStart.Format("20060102"))
	alert := Alert{
		ID: alertID, Metric: metric, Direction: direction, PctChange: round1(pct),
		WindowStart: windowStart.UTC().Format(time.RFC3339),
		WindowEnd:   windowEnd.UTC().Format(time.RFC3339),
		BaselineKind: baselineKind, Severity: severityFrom(math.Abs(pct)),
	}
	ruledOut := buildRuledOut(culprit, baseline, observed, pct)
	diagnosis := buildDiagnosis(alert, decomp, culprit, segments, ruledOut)
	mark("evidence", "Packaged evidence JSON with citations", t3)
	trace = append(trace, TraceStep{Step: "narrate", Detail: "Narration deferred to Node/Gemini from evidence only", DurationMs: 0})

	return &Investigation{
		ID: invID, Status: "complete", Alert: alert,
		Decomposition: decomp, Segments: segments, RuledOut: ruledOut,
		Diagnosis: diagnosis, Trace: trace,
	}, nil
}

func resolveWindow(startStr, endStr string) (time.Time, time.Time, error) {
	if startStr == "" || endStr == "" {
		start, _ := time.Parse(time.RFC3339, "2026-06-28T00:00:00Z")
		end, _ := time.Parse(time.RFC3339, "2026-06-28T23:59:59Z")
		return start, end, nil
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		start, err = time.Parse("2006-01-02", startStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		end, err = time.Parse("2006-01-02", endStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end = end.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	}
	return start.UTC(), end.UTC(), nil
}

func baselineRange(windowStart, windowEnd time.Time, kind string) (time.Time, time.Time) {
	day := time.Date(windowStart.Year(), windowStart.Month(), windowStart.Day(), 0, 0, 0, 0, time.UTC)
	switch kind {
	case "trailing_14d":
		return day.AddDate(0, 0, -14), day.Add(-time.Second)
	default: // trailing_7d
		return day.AddDate(0, 0, -7), day.Add(-time.Second)
	}
}

func querySameWeekdayBaseline(ctx context.Context, conn *chClient, windowStart, windowEnd time.Time, weeks int) (metricBag, error) {
	day := time.Date(windowStart.Year(), windowStart.Month(), windowStart.Day(), 0, 0, 0, 0, time.UTC)
	dur := windowEnd.Sub(windowStart)
	var sum metricBag
	n := 0
	for i := 1; i <= weeks; i++ {
		ws := day.AddDate(0, 0, -7*i)
		we := ws.Add(dur)
		m, err := queryMetrics(ctx, conn, ws, we)
		if err != nil {
			return metricBag{}, err
		}
		if m.Requests == 0 {
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
		return metricBag{}, fmt.Errorf("no same-weekday baseline days with data")
	}
	return scaleBag(sum, 1/float64(n)), nil
}

func queryMetrics(ctx context.Context, conn *chClient, start, end time.Time) (metricBag, error) {
	q := fmt.Sprintf(`
		SELECT
			count() AS requests,
			sum(is_filled) AS fills,
			sum(is_impression) AS impressions,
			sum(is_click) AS clicks,
			sum(revenue) AS revenue
		FROM ad_events
		WHERE event_time >= %s AND event_time <= %s
	`, quoteTime(start), quoteTime(end))
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

type dimSpec struct {
	Name string
	Expr string
	Join string
}

func rankSegments(ctx context.Context, conn *chClient, obsStart, obsEnd time.Time, baselineKind, culprit string) ([]Segment, error) {
	dims := []dimSpec{
		{Name: "ad_format", Expr: "e.ad_format"},
		{Name: "region", Expr: "g.region", Join: "LEFT JOIN geo_device g ON e.geo_device_id = g.geo_device_id"},
		{Name: "country", Expr: "g.country", Join: "LEFT JOIN geo_device g ON e.geo_device_id = g.geo_device_id"},
		{Name: "device_model", Expr: "g.device_model", Join: "LEFT JOIN geo_device g ON e.geo_device_id = g.geo_device_id"},
		{Name: "os_version", Expr: "g.os_version", Join: "LEFT JOIN geo_device g ON e.geo_device_id = g.geo_device_id"},
		{Name: "category", Expr: "a.category", Join: "LEFT JOIN apps a ON e.app_id = a.app_id"},
		{Name: "publisher_tier", Expr: "a.publisher_tier", Join: "LEFT JOIN apps a ON e.app_id = a.app_id"},
		{Name: "vertical", Expr: "adv.vertical", Join: "LEFT JOIN advertisers adv ON e.advertiser_id = adv.advertiser_id"},
		{Name: "campaign_type", Expr: "adv.campaign_type", Join: "LEFT JOIN advertisers adv ON e.advertiser_id = adv.advertiser_id"},
	}

	var all []Segment
	for _, d := range dims {
		segs, err := queryDimensionDelta(ctx, conn, d, obsStart, obsEnd, baselineKind, culprit)
		if err != nil {
			return nil, err
		}
		all = append(all, segs...)
	}
	sort.Slice(all, func(i, j int) bool {
		return math.Abs(all[i].ContributionPct) > math.Abs(all[j].ContributionPct)
	})
	if len(all) > 8 {
		all = all[:8]
	}
	return all, nil
}

func metricSQL(culprit string) (string, string) {
	switch culprit {
	case "requests":
		return "count()", "count()"
	case "fill_rate":
		return "sum(is_filled) / count()", "sum(is_filled)"
	case "render_rate":
		return "sum(is_impression) / nullIf(sum(is_filled),0)", "sum(is_impression)"
	case "ecpm":
		return "sum(revenue) / nullIf(sum(is_impression),0) * 1000", "sum(revenue)"
	case "ctr":
		return "sum(is_click) / nullIf(sum(is_impression),0)", "sum(is_click)"
	default:
		return "sum(revenue)", "sum(revenue)"
	}
}

func queryDimensionDelta(ctx context.Context, conn *chClient, d dimSpec, obsStart, obsEnd time.Time, baselineKind, culprit string) ([]Segment, error) {
	metricExpr, weightExpr := metricSQL(culprit)
	join := d.Join
	day := time.Date(obsStart.Year(), obsStart.Month(), obsStart.Day(), 0, 0, 0, 0, time.UTC)
	dur := obsEnd.Sub(obsStart)

	var basePred string
	switch baselineKind {
	case "trailing_7d":
		basePred = fmt.Sprintf("e.event_time >= %s AND e.event_time <= %s", quoteTime(day.AddDate(0, 0, -7)), quoteTime(day.Add(-time.Second)))
	case "trailing_14d":
		basePred = fmt.Sprintf("e.event_time >= %s AND e.event_time <= %s", quoteTime(day.AddDate(0, 0, -14)), quoteTime(day.Add(-time.Second)))
	default:
		parts := make([]string, 0, 3)
		for i := 1; i <= 3; i++ {
			ws := day.AddDate(0, 0, -7*i)
			we := ws.Add(dur)
			parts = append(parts, fmt.Sprintf("(e.event_time >= %s AND e.event_time <= %s)", quoteTime(ws), quoteTime(we)))
		}
		basePred = strings.Join(parts, " OR ")
	}

	q := fmt.Sprintf(`
		WITH
		obs AS (
			SELECT %s AS dim, %s AS metric, %s AS weight
			FROM ad_events e
			%s
			WHERE e.event_time >= %s AND e.event_time <= %s
			GROUP BY dim
		),
		base AS (
			SELECT dim, avg(metric) AS metric, avg(weight) AS weight
			FROM (
				SELECT %s AS dim, %s AS metric, %s AS weight, toDate(e.event_time) AS d
				FROM ad_events e
				%s
				WHERE %s
				GROUP BY dim, d
			)
			GROUP BY dim
		)
		SELECT
			toString(obs.dim) AS dim,
			obs.metric AS obs_metric,
			base.metric AS base_metric,
			obs.weight AS obs_weight,
			base.weight AS base_weight
		FROM obs
		INNER JOIN base ON obs.dim = base.dim
		WHERE obs.dim IS NOT NULL AND toString(obs.dim) != ''
		ORDER BY abs(obs.metric - base.metric) * abs(obs.weight) DESC
		LIMIT 5
	`, d.Expr, metricExpr, weightExpr, join, quoteTime(obsStart), quoteTime(obsEnd),
		d.Expr, metricExpr, weightExpr, join, basePred)

	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, err
	}

	var totalAbs float64
	type raw struct {
		dim                   string
		obsMetric, baseMetric float64
		obsWeight             float64
	}
	raws := make([]raw, 0, len(rows))
	for _, row := range rows {
		r := raw{
			dim: asString(row["dim"]), obsMetric: asFloat(row["obs_metric"]),
			baseMetric: asFloat(row["base_metric"]), obsWeight: asFloat(row["obs_weight"]),
		}
		contrib := math.Abs(r.obsMetric-r.baseMetric) * math.Max(math.Abs(r.obsWeight), 1)
		totalAbs += contrib
		raws = append(raws, r)
	}

	out := make([]Segment, 0, len(raws))
	for _, r := range raws {
		contrib := 0.0
		if totalAbs > 0 {
			contrib = math.Abs(r.obsMetric-r.baseMetric) * math.Max(math.Abs(r.obsWeight), 1) / totalAbs * 100
		}
		out = append(out, Segment{
			Dimension: d.Name, Value: r.dim, Metric: culprit,
			DeltaPct: round1(pctChange(r.obsMetric, r.baseMetric)), ContributionPct: round1(contrib),
		})
	}
	return out, nil
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
			Detail: "Same-weekday / trailing baseline applied; residual movement remains after seasonal adjustment.",
		})
	}
	return out
}

func buildDiagnosis(alert Alert, decomp []Factor, culprit string, segments []Segment, ruled []RuledOut) Diagnosis {
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

	metricLabel := humanMetric(alert.Metric)
	culpritLabel := humanMetric(culprit)
	text := fmt.Sprintf("%s %s %s, primarily driven by %s",
		metricLabel, alert.Direction, formatPct(math.Abs(alert.PctChange)), culpritLabel)
	if culpritFactor != nil {
		text += fmt.Sprintf(" (%s to %s, %s)",
			formatFactorValue(culprit, culpritFactor.Baseline),
			formatFactorValue(culprit, culpritFactor.Observed),
			formatSignedPct(culpritFactor.DeltaPct),
		)
	}
	if top != "" {
		text += ". Top segments: " + top + "."
	}
	if len(ruled) > 0 {
		text += " Ruled out: " + ruled[0].Reason + "."
	}

	citations := []Citation{{Label: metricLabel + " change", Value: formatSignedPct(alert.PctChange)}}
	if culpritFactor != nil {
		citations = append(citations, Citation{
			Label: culpritFactor.Label + " change",
			Value: formatSignedPct(culpritFactor.DeltaPct),
		})
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

func requestFromInvestigationID(id string) (InvestigateRequest, error) {
	// Expected: inv-{metric}-{YYYYMMDD} e.g. inv-revenue-20260626
	parts := strings.Split(id, "-")
	if len(parts) < 3 || parts[0] != "inv" {
		return InvestigateRequest{}, fmt.Errorf("invalid investigation id")
	}
	datePart := parts[len(parts)-1]
	metric := strings.Join(parts[1:len(parts)-1], "_")
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
		BaselineKind: "same_weekday_trailing",
	}, nil
}

func detectAlerts(ctx context.Context, conn *chClient, cache *invCache) ([]map[string]any, error) {
	rows, err := conn.QueryMaps(ctx, `
		SELECT toString(toDate(event_time)) AS d
		FROM ad_events
		GROUP BY d
		ORDER BY d
	`)
	if err != nil {
		return nil, err
	}
	days := make([]time.Time, 0, len(rows))
	for _, r := range rows {
		d, err := time.Parse("2006-01-02", asString(r["d"]))
		if err != nil {
			continue
		}
		days = append(days, d)
	}
	if len(days) < 15 {
		return []map[string]any{}, nil
	}

	out := []map[string]any{}
	startIdx := len(days) - 14
	if startIdx < 8 {
		startIdx = 8
	}
	for _, d := range days[startIdx:] {
		ws := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		we := ws.Add(24*time.Hour - time.Second)
		inv, err := runInvestigation(ctx, conn, InvestigateRequest{
			Metric: "revenue", WindowStart: ws.Format(time.RFC3339), WindowEnd: we.Format(time.RFC3339),
			BaselineKind: "same_weekday_trailing",
		})
		if err != nil {
			continue
		}
		if math.Abs(inv.Alert.PctChange) < 5 {
			continue
		}
		if cache != nil {
			cache.put(inv)
		}
		out = append(out, map[string]any{
			"id": inv.Alert.ID, "metric": inv.Alert.Metric, "direction": inv.Alert.Direction,
			"pctChange": inv.Alert.PctChange, "windowStart": inv.Alert.WindowStart, "windowEnd": inv.Alert.WindowEnd,
			"baselineKind": inv.Alert.BaselineKind, "severity": inv.Alert.Severity,
			"investigationId": inv.ID, "status": inv.Status, "summary": inv.Diagnosis.Text,
		})
		if len(out) >= 6 {
			break
		}
	}
	return out, nil
}
