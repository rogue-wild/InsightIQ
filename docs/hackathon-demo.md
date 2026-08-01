# Hackathon demo guide

## Elevator pitch (30 seconds)

> InsightIQ turns ClickHouse alerts into answers. We detect z-score anomalies on the live view layer, run a deterministic RCA in Go — baseline, metric tree, segments, seasonality, counterfactual — then Gemini narrates only those numbers. Every claim has a citation and an evidence hash you can export for the unseen incident.

## What to show mentors

1. **Dashboard** — pick 21 Jun 2026, filter India + iOS, show live totals  
2. **Alerts (Daily)** — open a critical revenue card (e.g. `adv_0000`)  
3. **Investigation** — culprit (e.g. requests), top segments, seasonality not a false alarm, counterfactual recovery %  
4. **Export unseen bundle** — show `evidence.hash` + `trace`  
5. **Chat** — natural language date + filters; answer cites human date and rounded revenue  
6. **Langfuse** (if keys set) — open the chat trace tree  

## Architecture talking points

- ClickHouse does the analysis; LLM is a narrator  
- View layer only (`metric_hourly_snapshot`, `alerts_live`, contributors) — no raw scans in the product path  
- Daily alerts = UX rollup of peak hour (honest about seasonality baseline)  
- Langfuse + LibreChat satisfy the “integrate at least one” requirement meaningfully  

## Unseen incident checklist

- [ ] Engine + API healthy against the new data  
- [ ] `node scripts/export-unseen.mjs --list` shows new UUIDs  
- [ ] Export JSON includes diagnosis, trace, evidence hash, seasonality, waterfall, counterfactual  
- [ ] Numbers in diagnosis match ClickHouse (spot-check one citation)  
- [ ] No mock ids (`alert-rev-…` / `inv-001`)  

## Judging alignment

| Criterion | Where we show it |
|-----------|------------------|
| Detection & localization | `alerts_live` + contributors / segments |
| Explanation trustworthiness | Citations + evidence hash; Gemini evidence-only prompt |
| ClickHouse depth | Engine SQL logs per request; view-layer RCA |
| Traceability | Investigation `trace` + Langfuse |
| Unseen incident | Export UI + CLI |

## Ports cheat sheet

| Service | URL |
|---------|-----|
| Web | http://localhost:5173 |
| API | http://localhost:4000 |
| Engine | http://localhost:4100 |
| LibreChat | http://localhost:3080 |

## Related docs

- [Architecture](./architecture.md)
- [Setup](./setup.md)
- [API](./api-reference.md)
- Problem statement: [`../PROBLEM_STATEMENT.md`](../PROBLEM_STATEMENT.md)
