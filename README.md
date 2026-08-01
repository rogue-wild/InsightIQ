# InsightIQ

ClickHouse-native autonomous analytics control plane: detect metric anomalies, isolate root causes, and narrate verified answers in plain English.

## Current status

- React dashboard (mock or live API)
- Go investigation engine on ClickHouse Cloud (`insightiq` view layer)
- Node API proxy + Gemini narration + LibreChat `/v1` endpoint

## Run

```bash
# Go engine (ClickHouse RCA)
cd apps/engine && go run .

# Node API
cd apps/api && npm run dev

# Dashboard + in-app chat
cd apps/web && npm run dev
```

- Engine: http://localhost:4100  
- API: http://localhost:4000  
- Web: http://localhost:5173 (`/dashboard` analytics, `/chat` RCA chat)  
- LibreChat (white-labeled InsightIQ + MCP): http://localhost:3080

Set `VITE_LIBRECHAT_URL=http://localhost:3080` and optionally `VITE_USE_MOCK=false` + `VITE_API_URL=http://localhost:4000` in `apps/web/.env`.

## Repo layout

```
apps/web/       Vite + React dashboard
apps/api/       Node API (REST, Gemini, LibreChat compatible)
apps/engine/    Go ClickHouse investigation engine
packages/contracts/
infra/librechat/
data/           ad_events.csv + dimension tables
```

## ClickHouse (InsightIQ)

Database `insightiq` — autonomous analytics control plane:

- `ad_events_raw` → `agg_hourly` → `metric_hourly_snapshot`
- `baseline_hourly` + z-score → `alerts_live`
- RCA: `alert_dimension_contributors`, `alert_observations`

Engine + MCP read the pre-computed view layer only (never scan raw events for UI/chat).
Credentials live in gitignored `apps/engine/.env` and `apps/api/.env`.

See [infra/clickhouse/insightiq_view_layer.sql](infra/clickhouse/insightiq_view_layer.sql).

## Contract

See [packages/contracts/investigation.schema.json](packages/contracts/investigation.schema.json).

## Unseen-incident export

From an investigation in the UI: **Export unseen bundle**.

Or CLI:

```bash
node scripts/export-unseen.mjs --alertId=<uuid> --out=./unseen-out
# or
node scripts/export-unseen.mjs --investigationId=inv-<uuid>
```

Writes `{id}-unseen-export.json` with diagnosis, immutable trace, evidence SHA-256, seasonality, waterfall, counterfactual, and hypotheses.