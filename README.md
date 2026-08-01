# AdInsight

Automated root-cause analyst for the Click-a-thon 2026 InMobi problem: detect metric moves, drill down in ClickHouse, and narrate an evidence-backed diagnosis.

## Current status

- React dashboard (mock or live API)
- Go investigation engine on ClickHouse Cloud (9M events loaded)
- Node API proxy + Gemini narration + LibreChat `/v1` endpoint

## Run

```bash
# Go engine (ClickHouse RCA)
cd apps/engine && go run .

# Node API
cd apps/api && npm run dev

# Dashboard
cd apps/web && npm run dev
```

- Engine: http://localhost:4100  
- API: http://localhost:4000  
- Web: http://localhost:5173  
- LibreChat: http://localhost:3080 (Ask in chat)

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

## ClickHouse

Database `adinsight` with `ad_events`, `apps`, `advertisers`, `geo_device`. Credentials live in gitignored `apps/engine/.env` and `apps/api/.env`.

## Contract

See [packages/contracts/investigation.schema.json](packages/contracts/investigation.schema.json).
