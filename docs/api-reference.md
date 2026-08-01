# API reference

Base URLs (local):

- Engine: `http://127.0.0.1:4100`
- API: `http://127.0.0.1:4000` (proxies engine + adds chat / narration)

---

## Engine (`apps/engine`)

### `GET /health`

```json
{ "ok": true, "service": "insightiq-engine", "database": "insightiq", "alerts": 388 }
```

### `GET /alerts?granularity=day|hour`

Default: **`day`**.

- `day` — one card per advertiser (+ day), using the peak hourly anomaly that day; windows expanded to full UTC day; `baselineKind: daily_peak_hour`
- `hour` — native hourly buckets from `alerts_live`

Returns an array of alert cards (id, metric, pctChange, window, severity, summary, categories, …).

### `POST /investigate`

Body (either):

```json
{ "alertId": "<uuid-from-alerts_live>" }
```

or window fields for custom investigate:

```json
{
  "alertId": "optional",
  "metric": "revenue",
  "windowStart": "2026-06-21T10:00:00Z",
  "windowEnd": "2026-06-21T10:59:59Z",
  "baselineKind": "same_hour_4w_seasonality"
}
```

Response: full investigation JSON (see [contracts](../packages/contracts/investigation.schema.json)).

### `GET /investigations/:id`

Cached investigation or rebuild (`inv-<alertUuid>`).

### `GET /investigations/:id/export`

Unseen-incident bundle: diagnosis, trace, evidence hash, seasonality, waterfall, counterfactual, hypotheses.

### `GET /dashboard/meta`

Metrics + dimensions catalog + `dataRange` `{ min, max, buckets }` (from `agg_hourly`).

### `POST /dashboard/query`

```json
{
  "start": "2026-06-21T00:00:00Z",
  "end": "2026-06-21T23:59:59Z",
  "granularity": "hour",
  "metrics": ["revenue", "requests", "fill_rate", "ecpm"],
  "dimensions": ["ad_format", "country"],
  "filters": { "country": ["IN"], "os_version": ["iOS 17.2"] },
  "compare": { "start": "...", "end": "..." },
  "limit": 10
}
```

### `GET /dashboard/filters?dimension=&start=&end=`

Distinct dimension values for filter pickers.

---

## Node API (`apps/api`)

### `GET /health`

Includes `engineUrl`, nested engine health, `gemini`, `langfuse`.

### Alerts / investigations / dashboard

| Method | Path | Notes |
|--------|------|--------|
| GET | `/api/alerts?granularity=day\|hour` | Proxies engine |
| GET | `/api/alerts/:alertId` | From list for that granularity |
| GET | `/api/alerts/:alertId/investigation` | Investigate-by-alert |
| GET | `/api/investigations/:id` | |
| GET | `/api/investigations/:id/export` | |
| POST | `/api/investigate` | |
| GET | `/api/dashboard/meta` | |
| POST | `/api/dashboard/query` | |
| GET | `/api/dashboard/filters` | |

**Live-only:** empty engine results return `[]` / `404` / `502` — no mock sample fallbacks.

### Chat (OpenAI-compatible)

#### `POST /v1/chat/completions`

```json
{
  "model": "insightiq-rca",
  "messages": [{ "role": "user", "content": "Revenue for India iOS 17.2 on 21 June 2026?" }],
  "stream": false,
  "investigationId": "optional",
  "alertId": "optional",
  "sessionId": "optional"
}
```

Behavior:

1. **Dashboard intent** — if the question mentions geo / OS / format / etc., run a dashboard query  
   - Dates: ISO (`2026-06-21`) or natural (`21 June 2026`, `June 21, 2026`)  
   - Else: latest day (narrow filters) or last 7 days (broad geo) from live `dataRange`
2. Else **investigation** — resolve alert/investigation id or default live investigation  
3. **Narrate** with Gemini from evidence JSON only (human dates, rounded numbers)

Also: `GET /v1/models`.

---

## Investigation JSON (shape)

Key fields:

| Field | Purpose |
|-------|---------|
| `alert` | Metric, window, severity, advertiser |
| `decomposition` | Revenue identity factors + culprit |
| `segments` | Top dimension drivers (capped) |
| `ruledOut` | What was checked and cleared |
| `diagnosis` | Short text + citations |
| `trace` | Ordered steps + durations |
| `seasonality` | Flat vs seasonal residual |
| `waterfall` | Contribution share |
| `counterfactual` | “If culprit held…” |
| `hypotheses` | Ranked confidence |
| `evidence.hash` | SHA-256 lock over sources |

Schema: [`packages/contracts/investigation.schema.json`](../packages/contracts/investigation.schema.json).
