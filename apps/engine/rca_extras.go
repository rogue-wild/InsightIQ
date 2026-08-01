package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type SeasonalityCheck struct {
	Status            string  `json:"status"` // residual_remains | ruled_out_as_seasonality | skipped
	FlatDeltaPct      float64 `json:"flatDeltaPct"`
	SeasonalDeltaPct  float64 `json:"seasonalDeltaPct"`
	Detail            string  `json:"detail"`
}

type WaterfallStep struct {
	Factor        string  `json:"factor"`
	Label         string  `json:"label"`
	RevenueImpact float64 `json:"revenueImpact"`
	SharePct      float64 `json:"sharePct"`
	Status        string  `json:"status"`
}

type Counterfactual struct {
	Culprit                string  `json:"culprit"`
	ObservedRevenue        float64 `json:"observedRevenue"`
	CounterfactualRevenue  float64 `json:"counterfactualRevenue"`
	RecoveredRevenue       float64 `json:"recoveredRevenue"`
	RecoveredPctOfGap      float64 `json:"recoveredPctOfGap"`
	Detail                 string  `json:"detail"`
}

type Hypothesis struct {
	Rank           int     `json:"rank"`
	Factor         string  `json:"factor"`
	Label          string  `json:"label"`
	ConfidencePct  float64 `json:"confidencePct"`
	Why            string  `json:"why"`
}

type EvidenceLock struct {
	Hash      string   `json:"hash"`
	Generated string   `json:"generatedAt"`
	Sources   []string `json:"sources"`
}

func revenueIdentity(m metricBag) float64 {
	return m.Requests * m.FillRate() * m.RenderRate() * (m.ECPM() / 1000)
}

// evaluateSeasonality compares a naive flat prior-day window vs the seasonal baseline.
// If the flat view looks anomalous but the seasonal residual is small, mark as seasonality (not an incident).
func evaluateSeasonality(metric string, observed, seasonal, flat metricBag, seasonalPct float64) SeasonalityCheck {
	obsV := metricValue(observed, metric)
	flatPct := pctChange(obsV, metricValue(flat, metric))
	seasPct := seasonalPct
	if math.Abs(seasPct) < 0.05 {
		seasPct = pctChange(obsV, metricValue(seasonal, metric))
	}

	if math.Abs(flatPct) >= 5 && math.Abs(seasPct) < 3 {
		return SeasonalityCheck{
			Status:           "ruled_out_as_seasonality",
			FlatDeltaPct:     round1(flatPct),
			SeasonalDeltaPct: round1(seasPct),
			Detail: fmt.Sprintf(
				"Vs a flat prior-day average this looked like %s, but vs same-hour × 4 weeks residual is only %s — consistent with seasonality, not a new incident.",
				formatSignedPct(round1(flatPct)), formatSignedPct(round1(seasPct)),
			),
		}
	}
	if math.Abs(seasPct) >= 5 {
		return SeasonalityCheck{
			Status:           "residual_remains",
			FlatDeltaPct:     round1(flatPct),
			SeasonalDeltaPct: round1(seasPct),
			Detail: fmt.Sprintf(
				"Same-hour × 4-week seasonality applied (flat prior-day was %s). Residual vs seasonal baseline is %s — still anomalous.",
				formatSignedPct(round1(flatPct)), formatSignedPct(round1(seasPct)),
			),
		}
	}
	return SeasonalityCheck{
		Status:           "skipped",
		FlatDeltaPct:     round1(flatPct),
		SeasonalDeltaPct: round1(seasPct),
		Detail:           "Movement within noise after seasonality adjustment.",
	}
}

func buildWaterfall(base, obs metricBag, decomp []Factor) []WaterfallStep {
	// Sequential swap along revenue identity to attribute $ impact.
	order := []string{"requests", "fill_rate", "render_rate", "ecpm"}
	labels := map[string]string{
		"requests": "Requests", "fill_rate": "Fill rate", "render_rate": "Render rate", "ecpm": "eCPM",
	}
	statusBy := map[string]string{}
	for _, f := range decomp {
		statusBy[f.Factor] = f.Status
	}

	cur := base
	baseRev := revenueIdentity(base)
	steps := make([]WaterfallStep, 0, len(order))
	impacts := make([]float64, 0, len(order))

	apply := func(bag metricBag, factor string) metricBag {
		out := bag
		switch factor {
		case "requests":
			out.Requests = obs.Requests
		case "fill_rate":
			out.Fills = obs.FillRate() * out.Requests
		case "render_rate":
			out.Impressions = obs.RenderRate() * out.Fills
		case "ecpm":
			if out.Impressions > 0 {
				out.Revenue = (obs.ECPM() / 1000) * out.Impressions
			}
		}
		return out
	}

	prevRev := baseRev
	for _, factor := range order {
		next := apply(cur, factor)
		// For fill/render, also keep revenue identity consistent for next steps
		if factor == "fill_rate" || factor == "render_rate" || factor == "requests" {
			next.Revenue = revenueIdentity(next)
		}
		if factor == "ecpm" {
			next.Revenue = (obs.ECPM() / 1000) * next.Impressions
		}
		rev := next.Revenue
		if factor != "ecpm" {
			rev = revenueIdentity(next)
		}
		impact := rev - prevRev
		impacts = append(impacts, impact)
		st := statusBy[factor]
		if st == "" {
			st = "neutral"
		}
		steps = append(steps, WaterfallStep{
			Factor: factor, Label: labels[factor],
			RevenueImpact: roundMoney(impact), Status: st,
		})
		cur = next
		prevRev = rev
	}

	totalAbs := 0.0
	for _, v := range impacts {
		totalAbs += math.Abs(v)
	}
	for i := range steps {
		if totalAbs > 0 {
			steps[i].SharePct = round1(math.Abs(impacts[i]) / totalAbs * 100)
		}
	}
	return steps
}

func buildCounterfactual(culprit string, base, obs metricBag) Counterfactual {
	cf := obs
	switch culprit {
	case "requests":
		cf.Requests = base.Requests
		cf.Fills = obs.FillRate() * cf.Requests
		cf.Impressions = obs.RenderRate() * cf.Fills
		cf.Revenue = (obs.ECPM() / 1000) * cf.Impressions
	case "fill_rate":
		cf.Fills = base.FillRate() * obs.Requests
		cf.Impressions = obs.RenderRate() * cf.Fills
		cf.Revenue = (obs.ECPM() / 1000) * cf.Impressions
	case "render_rate":
		cf.Impressions = base.RenderRate() * obs.Fills
		cf.Revenue = (obs.ECPM() / 1000) * cf.Impressions
	case "ecpm":
		cf.Revenue = (base.ECPM() / 1000) * obs.Impressions
	default:
		cf.Revenue = revenueIdentity(base)
	}

	obsRev := obs.Revenue
	if obsRev == 0 {
		obsRev = revenueIdentity(obs)
	}
	baseRev := base.Revenue
	if baseRev == 0 {
		baseRev = revenueIdentity(base)
	}
	cfRev := cf.Revenue
	gap := baseRev - obsRev
	recovered := cfRev - obsRev
	recPct := 0.0
	if math.Abs(gap) > 1e-9 {
		recPct = recovered / gap * 100
	}

	detail := fmt.Sprintf(
		"If %s had stayed at baseline, estimated revenue would be %s instead of %s (recover %s of the gap).",
		humanMetric(culprit), formatNumber(roundMoney(cfRev)), formatNumber(roundMoney(obsRev)), formatPct(round1(recPct)),
	)
	return Counterfactual{
		Culprit:               culprit,
		ObservedRevenue:       roundMoney(obsRev),
		CounterfactualRevenue: roundMoney(cfRev),
		RecoveredRevenue:      roundMoney(recovered),
		RecoveredPctOfGap:     round1(recPct),
		Detail:                detail,
	}
}

func buildHypotheses(decomp []Factor, culprit string, segments []Segment) []Hypothesis {
	type scored struct {
		factor string
		label  string
		score  float64
		status string
	}
	var items []scored
	for _, f := range decomp {
		if f.Factor == "ctr" {
			continue
		}
		items = append(items, scored{f.Factor, f.Label, math.Abs(f.DeltaPct), f.Status})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].score > items[j].score })
	total := 0.0
	for _, it := range items {
		total += it.score
	}
	out := make([]Hypothesis, 0, 2)
	for i, it := range items {
		if i >= 2 {
			break
		}
		conf := 0.0
		if total > 0 {
			conf = it.score / total * 100
		}
		why := fmt.Sprintf("%s moved %s vs baseline.", it.label, formatSignedPct(round1(pctChangeForFactor(decomp, it.factor))))
		if i == 0 && len(segments) > 0 {
			why += fmt.Sprintf(" Top slice: %s=%s (%s contribution).",
				humanDimension(segments[0].Dimension), segments[0].Value, formatPct(segments[0].ContributionPct))
		}
		if i == 1 {
			why += " Demoted because absolute % move is smaller than the primary factor."
		}
		if it.factor == culprit {
			why = "Selected as primary culprit from revenue-identity walk. " + why
		}
		out = append(out, Hypothesis{
			Rank: i + 1, Factor: it.factor, Label: it.label,
			ConfidencePct: round1(conf), Why: why,
		})
	}
	return out
}

func pctChangeForFactor(decomp []Factor, factor string) float64 {
	for _, f := range decomp {
		if f.Factor == factor {
			return f.DeltaPct
		}
	}
	return 0
}

func buildEvidenceLock(inv *Investigation) EvidenceLock {
	payload := map[string]any{
		"id":            inv.ID,
		"alert":         inv.Alert,
		"decomposition": inv.Decomposition,
		"segments":      inv.Segments,
		"ruledOut":      inv.RuledOut,
		"waterfall":     inv.Waterfall,
		"counterfactual": inv.Counterfactual,
		"seasonality":   inv.Seasonality,
		"hypotheses":    inv.Hypotheses,
		"diagnosis":     inv.Diagnosis,
		"trace":         inv.Trace,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	sources := []string{
		"insightiq.alerts_live",
		"insightiq.metric_hourly_snapshot",
		"insightiq.alert_dimension_contributors",
		"insightiq.alert_observations",
	}
	return EvidenceLock{
		Hash:      hex.EncodeToString(sum[:]),
		Generated: time.Now().UTC().Format(time.RFC3339),
		Sources:   sources,
	}
}

func roundMoney(v float64) float64 { return math.Round(v*10000) / 10000 }

func enrichRuledOutWithSeasonality(ruled []RuledOut, season SeasonalityCheck, culprit string, base, obs metricBag, revenuePct float64) []RuledOut {
	out := make([]RuledOut, 0, len(ruled)+2)
	// Replace generic seasonality entry with first-class check.
	for _, r := range ruled {
		if strings.EqualFold(r.Reason, "Seasonality check") {
			continue
		}
		out = append(out, r)
	}
	switch season.Status {
	case "ruled_out_as_seasonality":
		out = append([]RuledOut{{
			Reason: "Seasonality (not an incident)",
			Detail: season.Detail,
		}}, out...)
	case "residual_remains":
		out = append(out, RuledOut{
			Reason: "Seasonality checked",
			Detail: season.Detail,
		})
	}
	_ = culprit
	_ = base
	_ = obs
	_ = revenuePct
	return out
}

func appendCounterfactualCitation(d Diagnosis, cf Counterfactual) Diagnosis {
	d.Citations = append(d.Citations, Citation{
		Label: "Counterfactual recovery",
		Value: fmt.Sprintf("%s recovered (%s of gap) if %s held",
			formatNumber(cf.RecoveredRevenue), formatPct(cf.RecoveredPctOfGap), humanMetric(cf.Culprit)),
	})
	if cf.Culprit != "" && !strings.Contains(d.Text, "Counterfactual") {
		d.Text = strings.TrimSpace(d.Text) + fmt.Sprintf(
			" Counterfactual: holding %s recovers %s of the gap.",
			humanMetric(cf.Culprit), formatPct(cf.RecoveredPctOfGap),
		)
	}
	return d
}
