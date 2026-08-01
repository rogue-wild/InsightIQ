# InsightIQ

ClickHouse-native autonomous analytics control plane for **Click-a-thon 2026 (InMobi)**: detect metric anomalies, isolate root causes, and narrate verified answers in plain English.

**Full documentation:** [docs/README.md](docs/README.md)

## Quick start

```bash
# Terminal 1 — Go engine (ClickHouse RCA)
cd apps/engine && go build -o bin/engine . && ./bin/engine

# Terminal 2 — Node API
cd apps/api && npm install && npm run dev

# Terminal 3 — Web
cd apps/web && npm install && npm run dev
```

| Service | URL |
|---------|-----|
| Web | http://localhost:5173 |
| API | http://localhost:4000 |
| Engine | http://localhost:4100 |
| LibreChat (optional) | http://localhost:3080 |

Web env (`apps/web/.env`):

```bash
VITE_API_URL=http://localhost:4000
VITE_LIBRECHAT_URL=http://localhost:3080
```

Engine / API ClickHouse + Gemini + Langfuse keys: see [docs/setup.md](docs/setup.md).

## What it does

1. **Detect** — z-score alerts from `alerts_live` (Daily or Hourly wall)  
2. **Drill down** — Go engine: baseline → metric tree → segments → seasonality / counterfactual  
3. **Narrate** — Gemini explains evidence JSON only (no invented numbers)  
4. **Export** — unseen-incident bundle with immutable trace + evidence SHA-256  

## Repo layout

```
apps/web/          React dashboard, alerts, investigation, chat
apps/api/          REST + Gemini + LibreChat /v1 + Langfuse
apps/engine/       Go ClickHouse investigation engine
packages/contracts Investigation schema
infra/clickhouse/  View-layer SQL reference
infra/librechat/   LibreChat white-label
docs/              Architecture, setup, API, demo guide
scripts/           Unseen export CLI
```

## Docs index

| Doc | Description |
|-----|-------------|
| [docs/architecture.md](docs/architecture.md) | System design |
| [docs/setup.md](docs/setup.md) | Run locally |
| [docs/data-model.md](docs/data-model.md) | ClickHouse tables / MVs |
| [docs/api-reference.md](docs/api-reference.md) | Endpoints |
| [docs/product-guide.md](docs/product-guide.md) | UI behavior |
| [docs/hackathon-demo.md](docs/hackathon-demo.md) | Mentor / judging pitch |

Also: [PROBLEM_STATEMENT.md](PROBLEM_STATEMENT.md), [metrics_glossary.md](metrics_glossary.md).

## Unseen-incident export

UI: Investigation → **Export unseen bundle**

```bash
node scripts/export-unseen.mjs --list
node scripts/export-unseen.mjs --alertId=<UUID> --out=./unseen-out
```
