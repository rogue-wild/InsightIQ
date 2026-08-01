# Architecture

## One-liner

InsightIQ is a **ClickHouse-native RCA control plane**: detect ad-metric anomalies, attribute them to dimensions, and narrate verified answers in plain English — with evidence, not vibes.

## System diagram

```mermaid
flowchart LR
  subgraph UI["apps/web · React / Vite :5173"]
    Dash[Dashboard]
    Alerts[Alert Wall]
    Inv[Investigation]
    Chat[In-app Chat]
  end

  subgraph API["apps/api · Node :4000"]
    REST[REST proxy]
    LLM[Gemini narration]
    OAI["/v1 chat completions"]
    Trace[Langfuse]
  end

  subgraph Engine["apps/engine · Go :4100"]
    Detect[Alert detect]
    RCA[Investigate / decompose / slice]
    DashQ[Dashboard query]
  end

  subgraph CH["ClickHouse Cloud · insightiq"]
    Snap[metric_hourly_snapshot]
    AlertsLive[alerts_live]
    Contrib[alert_dimension_contributors]
    Obs[alert_observations]
  end

  Dash & Alerts & Inv & Chat --> REST
  Chat --> OAI
  REST --> Detect & RCA & DashQ
  OAI --> LLM
  OAI --> DashQ
  OAI --> RCA
  Detect & RCA & DashQ --> Snap & AlertsLive & Contrib & Obs
  REST & OAI --> Trace
```

## Layers

### 1. Data plane — ClickHouse (`insightiq`)

Pre-aggregated **view layer** only for product paths (UI / chat / investigate never scan `ad_events_raw`):

| Object | Role |
|--------|------|
| `ad_events_raw` | Ingest facts (~9M) |
| `mv_hourly` | Materialized view → `agg_hourly` |
| `agg_hourly` | Hourly SummingMergeTree rollup |
| `metric_hourly_snapshot` | **VIEW** over `agg_hourly` (+ fill_rate, ctr, ecpm, rpr) |
| `baseline_hourly` | Expected / stddev per advertiser × metric × hour |
| `alerts_live` | Z-score anomalies (`\|z\| > 3`) |
| `alert_dimension_contributors` | Segment attribution |
| `alert_observations` | Ordered plain-English notes |
| `alert_rules` | Detection policy (`threshold`, `consecutive_buckets`, …) |

Details: [data-model.md](./data-model.md).

### 2. Investigation engine — Go (`:4100`)

Owns ClickHouse SQL and deterministic RCA:

- List alerts (`day` or `hour` granularity)
- Investigate: baseline → revenue-identity decompose → dimension slice → seasonality / waterfall / counterfactual / hypotheses
- Dashboard meta + filtered timeseries
- Unseen-incident export bundle (diagnosis + immutable trace + evidence SHA-256)

### 3. API — Node (`:4000`)

BFF between UI and engine:

- Proxies alerts, investigations, dashboard
- **Gemini narrates structured evidence only** (no raw event dumps)
- OpenAI-compatible `/v1/chat/completions` for in-app chat + LibreChat
- Langfuse traces chat / retrieve / generation

### 4. Web — React (`:5173`)

| Route | Purpose |
|-------|---------|
| `/` | Analytics dashboard |
| `/alerts` | Alert wall (Daily / Hourly) |
| `/investigations/:id` | RCA workspace + export |
| `/chat` | In-app NL Q&A |

Optional LibreChat shell: `:3080` (same API endpoint).

## Request paths

1. **Alert wall** — Web → API → Engine → `alerts_live` (+ batch contributors)  
2. **Open alert** — Web → API → Engine `/investigate` → contributors + observations + snapshot math → diagnosis JSON  
3. **“How is APAC?” chat** — API detects filters/dates → Engine dashboard query → Gemini narrates totals/deltas  
4. **“Why did revenue drop?” chat** — Resolve investigation → narrate from evidence + citations  

## Design principles

| Principle | Meaning |
|-----------|---------|
| Compute near the data | Heavy RCA in Go / ClickHouse, not in the browser or LLM |
| Evidence-bound LLM | Model explains numbers the engine already computed |
| View layer only | Product paths never scan raw events |
| Traceability | Investigation `trace` + Langfuse + evidence hash |
| Unseen-incident ready | Exportable bundle for judging |

## Repo map

```
apps/web/                 Vite + React UI
apps/api/                 Node BFF + Gemini + chat + Langfuse
apps/engine/              Go ClickHouse RCA engine
packages/contracts/       Investigation JSON schema
infra/clickhouse/         View-layer SQL reference
infra/librechat/          LibreChat white-label config
scripts/export-unseen.mjs Unseen-incident CLI
docs/                     This documentation
```
