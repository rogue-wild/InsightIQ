# API reference

Local bases:

- Engine: `http://127.0.0.1:4100`
- API: `http://127.0.0.1:4000`

---

## Engine

### `GET /health`

```json
{ "ok": true, "service": "insightiq-engine", "database": "insightiq", "alerts": 388 }
```

### `GET /alerts?granularity=day|hour`

Default: `day`.

- `day` — peak hourly anomaly per advertiser per UTC day
- `hour` — native hourly buckets from `alerts_live`

### `POST /investigate`

```json
{ "alertId": "<uuid>" }
```

Or window fields:

```json
{
  "metric": "revenue",
  "windowStart": "2026-06-21T10:00:00Z",
  "windowEnd": "2026-06-21T10:59:59Z",
  "baselineKind": "same_hour_4w_seasonality"
}
```

### `GET /investigations/:id`

Cached investigation or rebuild (`inv-<alertUuid>`).

### `GET /investigations/:id/export`

Evidence bundle: diagnosis, trace, evidence hash, seasonality, waterfall, counterfactual, hypotheses.

### `GET /dashboard/meta`

Metrics, dimensions, and `dataRange` `{ min, max, buckets }`.

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

Distinct values for filter pickers.

---

## Node API

### `GET /health`

Includes nested engine health, `gemini`, `langfuse`.

| Method | Path |
|--------|------|
| GET | `/api/alerts?granularity=day\|hour` |
| GET | `/api/alerts/:alertId` |
| GET | `/api/alerts/:alertId/investigation` |
| GET | `/api/investigations/:id` |
| GET | `/api/investigations/:id/export` |
| POST | `/api/investigate` |
| GET | `/api/dashboard/meta` |
| POST | `/api/dashboard/query` |
| GET | `/api/dashboard/filters` |

### `POST /v1/chat/completions`

```json
{
  "model": "insightiq-rca",
  "messages": [{ "role": "user", "content": "…" }],
  "stream": false,
  "investigationId": "optional",
  "alertId": "optional",
  "sessionId": "optional"
}
```

1. Dashboard intent when geo/OS/format filters are detected (optional explicit dates)
2. Otherwise resolve an investigation
3. Narrate from evidence only

Also: `GET /v1/models`.

## Investigation response fields

| Field | Purpose |
|-------|---------|
| `alert` | Metric, window, severity |
| `decomposition` | Revenue-identity factors |
| `segments` | Top dimension drivers |
| `ruledOut` | Checked and cleared factors |
| `diagnosis` | Short text + citations |
| `trace` | Ordered steps |
| `seasonality` / `waterfall` / `counterfactual` / `hypotheses` | RCA extras |
| `evidence.hash` | Content hash over sources |

Schema: [`packages/contracts/investigation.schema.json`](../packages/contracts/investigation.schema.json).
