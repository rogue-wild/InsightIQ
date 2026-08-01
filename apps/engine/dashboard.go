package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

type DashboardFilter map[string][]string

type DashboardRequest struct {
	Start       string          `json:"start"`
	End         string          `json:"end"`
	Granularity string          `json:"granularity"` // hour | day
	Metrics     []string        `json:"metrics"`
	Dimensions  []string        `json:"dimensions"`
	Filters     DashboardFilter `json:"filters"`
	Compare     *struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"compare"`
	Limit int `json:"limit"`
}

var dashboardMetricDefs = map[string]struct {
	Label string
	Expr  string // aggregate expression aliasing as metric name
}{
	"revenue":     {Label: "Revenue", Expr: "sum(revenue)"},
	"requests":    {Label: "Requests", Expr: "sum(requests)"},
	"impressions": {Label: "Impressions", Expr: "sum(impressions)"},
	"clicks":      {Label: "Clicks", Expr: "sum(clicks)"},
	"fills":       {Label: "Fills", Expr: "sum(fills)"},
	"fill_rate":   {Label: "Fill rate", Expr: "sum(fills) / nullIf(sum(requests), 0)"},
	"ctr":         {Label: "CTR", Expr: "sum(clicks) / nullIf(sum(impressions), 0)"},
	"ecpm":        {Label: "eCPM", Expr: "sum(revenue) / nullIf(sum(impressions), 0) * 1000"},
	"rpr":         {Label: "RPR", Expr: "sum(revenue) / nullIf(sum(requests), 0)"},
}

var dashboardDimensions = []struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}{
	{ID: "ad_format", Label: "Ad format"},
	{ID: "region", Label: "Region"},
	{ID: "country", Label: "Country"},
	{ID: "os_version", Label: "OS"},
	{ID: "campaign_type", Label: "Campaign type"},
	{ID: "publisher_tier", Label: "Publisher tier"},
	{ID: "category", Label: "Category"},
	{ID: "vertical", Label: "Vertical"},
}

func dashboardMeta() map[string]any {
	metrics := make([]map[string]string, 0, len(dashboardMetricDefs))
	for id, def := range dashboardMetricDefs {
		metrics = append(metrics, map[string]string{"id": id, "label": def.Label})
	}
	sort.Slice(metrics, func(i, j int) bool { return metrics[i]["id"] < metrics[j]["id"] })
	dims := make([]map[string]string, 0, len(dashboardDimensions))
	for _, d := range dashboardDimensions {
		dims = append(dims, map[string]string{"id": d.ID, "label": d.Label})
	}
	return map[string]any{"metrics": metrics, "dimensions": dims}
}

func queryDataRange(ctx context.Context, conn *chClient) (map[string]any, error) {
	// Prefer agg_hourly (physical SummingMergeTree) over metric_hourly_snapshot (VIEW).
	// Full-scan min/max/count on the view was timing out once the demo load landed (~9M rows).
	rows, err := conn.QueryMaps(ctx, `
		SELECT
			min(bucket) AS min_bucket,
			max(bucket) AS max_bucket,
			count() AS buckets
		FROM agg_hourly
	`)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return map[string]any{"min": nil, "max": nil, "buckets": 0}, nil
	}
	buckets := int64(asFloat(rows[0]["buckets"]))
	if buckets == 0 {
		return map[string]any{"min": nil, "max": nil, "buckets": 0}, nil
	}
	minB, _ := parseCHTime(asString(rows[0]["min_bucket"]))
	maxB, _ := parseCHTime(asString(rows[0]["max_bucket"]))
	// ClickHouse min/max on empty aggregates can surface epoch; treat as missing.
	if minB.Year() < 2000 || maxB.Year() < 2000 {
		return map[string]any{"min": nil, "max": nil, "buckets": buckets}, nil
	}
	end := maxB.UTC().Truncate(time.Hour).Add(time.Hour - time.Second)
	return map[string]any{
		"min":     minB.UTC().Format(time.RFC3339),
		"max":     end.Format(time.RFC3339),
		"buckets": buckets,
	}, nil
}

func runDashboard(ctx context.Context, conn *chClient, req DashboardRequest) (map[string]any, error) {
	start, end, err := resolveDashboardWindow(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	granularity := req.Granularity
	if granularity != "day" {
		granularity = "hour"
	}
	metrics := normalizeMetrics(req.Metrics)
	dims := normalizeDimensions(req.Dimensions)
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	filters := sanitizeFilters(req.Filters)

	currentTS, currentTotals, err := queryTimeseries(ctx, conn, start, end, granularity, metrics, filters)
	if err != nil {
		return nil, err
	}

	tables := map[string]any{}
	for _, dim := range dims {
		rows, err := queryDimensionTable(ctx, conn, start, end, dim, metrics, filters, limit, nil, nil)
		if err != nil {
			return nil, err
		}
		tables[dim] = rows
	}

	out := map[string]any{
		"start":       start.UTC().Format(time.RFC3339),
		"end":         end.UTC().Format(time.RFC3339),
		"granularity": granularity,
		"metrics":     metrics,
		"dimensions":  dims,
		"filters":     filters,
		"timeseries":  currentTS,
		"totals":      currentTotals,
		"tables":      tables,
	}

	if req.Compare != nil && req.Compare.Start != "" && req.Compare.End != "" {
		cStart, cEnd, err := resolveDashboardWindow(req.Compare.Start, req.Compare.End)
		if err != nil {
			return nil, err
		}
		compareTS, compareTotals, err := queryTimeseries(ctx, conn, cStart, cEnd, granularity, metrics, filters)
		if err != nil {
			return nil, err
		}
		out["compareStart"] = cStart.UTC().Format(time.RFC3339)
		out["compareEnd"] = cEnd.UTC().Format(time.RFC3339)
		out["compareTimeseries"] = compareTS
		out["compareTotals"] = compareTotals
		out["deltas"] = computeTotalsDelta(currentTotals, compareTotals)

		for _, dim := range dims {
			rows, err := queryDimensionTable(ctx, conn, start, end, dim, metrics, filters, limit, &cStart, &cEnd)
			if err != nil {
				return nil, err
			}
			tables[dim] = rows
		}
		out["tables"] = tables
	}

	return out, nil
}

func queryFilterValues(ctx context.Context, conn *chClient, dimension, startStr, endStr string, filters DashboardFilter) ([]string, error) {
	dimOK := false
	for _, d := range dashboardDimensions {
		if d.ID == dimension {
			dimOK = true
			break
		}
	}
	if !dimOK {
		return nil, fmt.Errorf("unsupported dimension")
	}
	start, end, err := resolveDashboardWindow(startStr, endStr)
	if err != nil {
		return nil, err
	}
	where := []string{
		fmt.Sprintf("bucket >= %s", quoteTime(start)),
		fmt.Sprintf("bucket <= %s", quoteTime(end)),
		fmt.Sprintf("%s != ''", dimension),
	}
	where = append(where, filterPredicates(sanitizeFilters(filters), dimension)...)
	q := fmt.Sprintf(`
		SELECT DISTINCT toString(%s) AS value
		FROM metric_hourly_snapshot
		WHERE %s
		ORDER BY value
		LIMIT 200
	`, dimension, strings.Join(where, " AND "))
	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		v := asString(r["value"])
		if v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

func resolveDashboardWindow(startStr, endStr string) (time.Time, time.Time, error) {
	if startStr == "" || endStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("start and end are required")
	}
	start, err := parseFlexibleTime(startStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := parseFlexibleTime(endStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !strings.Contains(endStr, "T") && !strings.Contains(endStr, " ") {
		end = end.Add(24*time.Hour - time.Second)
	}
	return start.UTC(), end.UTC(), nil
}

func normalizeMetrics(in []string) []string {
	if len(in) == 0 {
		return []string{"revenue", "requests", "fill_rate", "ecpm"}
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, m := range in {
		m = strings.TrimSpace(m)
		if _, ok := dashboardMetricDefs[m]; !ok || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	if len(out) == 0 {
		return []string{"revenue", "requests"}
	}
	return out
}

func normalizeDimensions(in []string) []string {
	allowed := map[string]bool{}
	for _, d := range dashboardDimensions {
		allowed[d.ID] = true
	}
	if len(in) == 0 {
		return []string{"ad_format", "country", "os_version", "campaign_type", "publisher_tier"}
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, d := range in {
		d = strings.TrimSpace(d)
		if !allowed[d] || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func sanitizeFilters(in DashboardFilter) DashboardFilter {
	if in == nil {
		return DashboardFilter{}
	}
	allowed := map[string]bool{}
	for _, d := range dashboardDimensions {
		allowed[d.ID] = true
	}
	out := DashboardFilter{}
	for k, vals := range in {
		if !allowed[k] || len(vals) == 0 {
			continue
		}
		clean := make([]string, 0, len(vals))
		seen := map[string]bool{}
		for _, v := range vals {
			v = strings.TrimSpace(v)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			clean = append(clean, v)
		}
		if len(clean) > 0 {
			out[k] = clean
		}
	}
	return out
}

func filterPredicates(filters DashboardFilter, excludeDim string) []string {
	preds := []string{}
	for dim, vals := range filters {
		if dim == excludeDim || len(vals) == 0 {
			continue
		}
		quoted := make([]string, 0, len(vals))
		for _, v := range vals {
			quoted = append(quoted, quoteString(v))
		}
		preds = append(preds, fmt.Sprintf("%s IN (%s)", dim, strings.Join(quoted, ",")))
	}
	return preds
}

func metricSelectSQL(metrics []string) string {
	parts := make([]string, 0, len(metrics))
	for _, m := range metrics {
		def := dashboardMetricDefs[m]
		// Alias must not collide with table columns (revenue, requests, fill_rate, …)
		// or ClickHouse rewrites later exprs like sum(requests) into sum(<alias>).
		parts = append(parts, fmt.Sprintf("%s AS %s", def.Expr, metricAlias(m)))
	}
	return strings.Join(parts, ",\n\t\t\t")
}

func metricAlias(metric string) string {
	return "m_" + metric
}

func metricOrderExpr(metric string) string {
	def, ok := dashboardMetricDefs[metric]
	if !ok {
		return metricAlias(metric)
	}
	return def.Expr
}

func readMetric(row map[string]any, metric string) float64 {
	if v, ok := row[metricAlias(metric)]; ok {
		return asFloat(v)
	}
	return asFloat(row[metric])
}

func queryTimeseries(ctx context.Context, conn *chClient, start, end time.Time, granularity string, metrics []string, filters DashboardFilter) ([]map[string]any, map[string]float64, error) {
	bucketExpr := "toStartOfHour(bucket)"
	if granularity == "day" {
		bucketExpr = "toStartOfDay(bucket)"
	}
	where := []string{
		fmt.Sprintf("bucket >= %s", quoteTime(start)),
		fmt.Sprintf("bucket <= %s", quoteTime(end)),
	}
	where = append(where, filterPredicates(filters, "")...)

	q := fmt.Sprintf(`
		SELECT
			toString(%s) AS t,
			%s
		FROM metric_hourly_snapshot
		WHERE %s
		GROUP BY t
		ORDER BY t
	`, bucketExpr, metricSelectSQL(metrics), strings.Join(where, " AND "))

	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, nil, err
	}

	totalsQ := fmt.Sprintf(`
		SELECT %s
		FROM metric_hourly_snapshot
		WHERE %s
	`, metricSelectSQL(metrics), strings.Join(where, " AND "))
	totalRows, err := conn.QueryMaps(ctx, totalsQ)
	if err != nil {
		return nil, nil, err
	}
	totals := map[string]float64{}
	if len(totalRows) > 0 {
		for _, m := range metrics {
			totals[m] = readMetric(totalRows[0], m)
		}
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		point := map[string]any{"t": asString(r["t"])}
		for _, m := range metrics {
			point[m] = readMetric(r, m)
		}
		out = append(out, point)
	}
	return out, totals, nil
}

func queryDimensionTable(
	ctx context.Context,
	conn *chClient,
	start, end time.Time,
	dimension string,
	metrics []string,
	filters DashboardFilter,
	limit int,
	compareStart, compareEnd *time.Time,
) ([]map[string]any, error) {
	where := []string{
		fmt.Sprintf("bucket >= %s", quoteTime(start)),
		fmt.Sprintf("bucket <= %s", quoteTime(end)),
		fmt.Sprintf("%s != ''", dimension),
	}
	where = append(where, filterPredicates(filters, dimension)...)

	primaryMetric := metrics[0]
	q := fmt.Sprintf(`
		SELECT
			toString(%s) AS dim_value,
			%s
		FROM metric_hourly_snapshot
		WHERE %s
		GROUP BY dim_value
		ORDER BY %s DESC
		LIMIT %d
	`, dimension, metricSelectSQL(metrics), strings.Join(where, " AND "), metricOrderExpr(primaryMetric), limit)

	rows, err := conn.QueryMaps(ctx, q)
	if err != nil {
		return nil, err
	}

	var compareMap map[string]map[string]float64
	if compareStart != nil && compareEnd != nil {
		cWhere := []string{
			fmt.Sprintf("bucket >= %s", quoteTime(*compareStart)),
			fmt.Sprintf("bucket <= %s", quoteTime(*compareEnd)),
			fmt.Sprintf("%s != ''", dimension),
		}
		cWhere = append(cWhere, filterPredicates(filters, dimension)...)
		cq := fmt.Sprintf(`
			SELECT
				toString(%s) AS dim_value,
				%s
			FROM metric_hourly_snapshot
			WHERE %s
			GROUP BY dim_value
		`, dimension, metricSelectSQL(metrics), strings.Join(cWhere, " AND "))
		cRows, err := conn.QueryMaps(ctx, cq)
		if err != nil {
			return nil, err
		}
		compareMap = map[string]map[string]float64{}
		for _, r := range cRows {
			vals := map[string]float64{}
			for _, m := range metrics {
				vals[m] = readMetric(r, m)
			}
			compareMap[asString(r["dim_value"])] = vals
		}
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		value := asString(r["dim_value"])
		row := map[string]any{"value": value}
		for _, m := range metrics {
			cur := readMetric(r, m)
			row[m] = cur
			if compareMap != nil {
				prev := 0.0
				if p, ok := compareMap[value]; ok {
					prev = p[m]
				}
				row[m+"_prev"] = prev
				row[m+"_delta"] = cur - prev
				row[m+"_delta_pct"] = round1(pctChange(cur, prev))
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func computeTotalsDelta(current, compare map[string]float64) map[string]any {
	out := map[string]any{}
	for k, cur := range current {
		prev := compare[k]
		out[k] = map[string]float64{
			"current":  cur,
			"previous": prev,
			"delta":    cur - prev,
			"deltaPct": round1(pctChange(cur, prev)),
		}
	}
	return out
}

func handleDashboardMeta(conn *chClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := dashboardMeta()
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if rng, err := queryDataRange(ctx, conn); err == nil {
			out["dataRange"] = rng
		} else {
			log.Printf("dashboard dataRange: %v", err)
			out["dataRange"] = map[string]any{"min": nil, "max": nil, "buckets": 0, "error": err.Error()}
		}
		writeJSON(w, out)
	}
}

func handleDashboardQuery(conn *chClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DashboardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		out, err := runDashboard(ctx, conn, req)
		if err != nil {
			log.Printf("dashboard error: %v", err)
			http.Error(w, `{"error":"dashboard_failed","detail":"`+escape(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, out)
	}
}

func handleDashboardFilters(conn *chClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dim := r.URL.Query().Get("dimension")
		start := r.URL.Query().Get("start")
		end := r.URL.Query().Get("end")
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		vals, err := queryFilterValues(ctx, conn, dim, start, end, nil)
		if err != nil {
			http.Error(w, `{"error":"filters_failed","detail":"`+escape(err.Error())+`"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"dimension": dim, "values": vals})
	}
}
