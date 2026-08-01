# Product guide (UI)

App shell: Dashboard · Alerts · Chat · Investigation.

## Dashboard (`/`)

- Live metrics from `metric_hourly_snapshot` via `/api/dashboard/query`
- Date presets + custom range; compare prior equal-length window
- Filters: region, country, OS, ad format, campaign, publisher tier, …
- Charts + dimension breakdown tables for selected metrics

## Alerts (`/alerts`)

### Granularity toggle

| Mode | Meaning |
|------|---------|
| **Daily** (default) | One card per advertiser per day = peak hourly anomaly that day |
| **Hourly** | Native hour buckets from `alerts_live` |

Daily links open investigations with `?view=day` so the header shows:

`21 Jun 2026 · peak hour 10:00 UTC`  
`daily peak hour · vs same hour, prior 4 weeks`

RCA math still runs on the **peak hour** (that is the real anomaly bucket).

### Category tabs

Geo / OS / Campaign / Ad format / Publisher / **Content** count alerts that already have a **contributor** in that dimension family.

If ClickHouse only has `vertical=entertainment` contributors, Content may be `1` while Geo/OS stay `0`. **All** still lists every wall card (including alerts without attribution yet).

### Baseline labels

| Label | Meaning |
|-------|---------|
| vs same hour, prior 4 weeks | Expected ≈ same hour-of-day over ~4 weeks (seasonality-aware) |
| daily peak hour | Daily card is a rollup; peak hour used that seasonal baseline |

## Investigation (`/investigations/:id`)

- Metric tree + contribution waterfall
- Seasonality panel (flat prior-day vs seasonal residual)
- Diagnosis (short) + citations (evidence-bound)
- Segment table, ruled-out list, hypotheses, counterfactual
- Trace timeline
- **Export unseen bundle** (JSON for judges)
- **Ask in chat** deep-link with investigation context

## Chat (`/chat`)

In-app NL interface → `POST /v1/chat/completions`.

Examples:

- `How is APAC revenue this week?` → dashboard query on latest live week  
- `What is the revenue for India for iOS 17.2 on 21 June 2026?` → filtered day query  
- `Why did revenue drop for inv-…?` → investigation narration  

**Tables:** Markdown tables render if the model emits them.  
**Charts:** not in chat yet — use Dashboard for graphs.  
**LibreChat:** optional shell at `VITE_LIBRECHAT_URL` using the same API.

## Design constraints (hackathon)

- Live ClickHouse data only (no mock alert wall)
- LLM never invents numbers — engine computes, Gemini narrates
- Prefer short diagnosis copy for mentor-facing demos
