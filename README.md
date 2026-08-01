# InsightIQ

ClickHouse-native analytics control plane: detect metric anomalies, isolate root causes, and narrate evidence-backed answers in plain English.

**Documentation:** [docs/README.md](docs/README.md)

## Quick start

```bash
# Terminal 1 — Engine
cd apps/engine && go build -o bin/engine . && ./bin/engine

# Terminal 2 — API
cd apps/api && npm install && npm run dev

# Terminal 3 — Web
cd apps/web && npm install && npm run dev
```

| Service | URL |
|---------|-----|
| Web | http://localhost:5173 |
| API | http://localhost:4000 |
| Engine | http://localhost:4100 |

`apps/web/.env`:

```bash
VITE_API_URL=http://localhost:4000
VITE_LIBRECHAT_URL=http://localhost:3080
```

See [docs/setup.md](docs/setup.md) for ClickHouse, Gemini, and Langfuse configuration.

## Capabilities

1. **Detect** — z-score alerts from `alerts_live` (daily or hourly wall)
2. **Investigate** — baseline, metric tree, segments, seasonality, counterfactual
3. **Narrate** — LLM explains engine evidence only
4. **Export** — investigation bundle with trace and evidence hash

## Repository

```
apps/web/          Dashboard, alerts, investigation, chat
apps/api/          REST, Gemini, OpenAI-compatible chat, Langfuse
apps/engine/       Go ClickHouse investigation engine
packages/contracts Investigation schema
infra/clickhouse/  View-layer SQL reference
infra/librechat/   LibreChat config
docs/              Project documentation
scripts/           Investigation export CLI
```

| Doc | Description |
|-----|-------------|
| [docs/architecture.md](docs/architecture.md) | System design |
| [docs/setup.md](docs/setup.md) | Local setup |
| [docs/data-model.md](docs/data-model.md) | ClickHouse schema |
| [docs/api-reference.md](docs/api-reference.md) | HTTP APIs |
| [docs/product-guide.md](docs/product-guide.md) | UI behavior |

## Export an investigation

UI: Investigation → **Export**

```bash
node scripts/export-investigation.mjs --list
node scripts/export-investigation.mjs --alertId=<UUID> --out=./exports
```
