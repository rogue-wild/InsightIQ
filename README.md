# InsightIQ

ClickHouse-native anomaly detection and automated root-cause analysis for large-scale ad-tech event streams. A reactive cascade inside ClickHouse turns raw events into noise-filtered alerts with dimension-level attribution; the app layer investigates and narrates that evidence in plain English.

**Documentation:** [docs/README.md](docs/README.md) · **Native pipeline:** [docs/pipeline.md](docs/pipeline.md)

## Why it matters

Traditional observability often pays high compute/egress costs and suffers alert fatigue from low-volume noise. InsightIQ keeps aggregation, seasonality baselines, Z-score detection, and first-pass multi-dimensional attribution **inside ClickHouse**, then surfaces only high-significance incidents to the UI.

In the reference dataset, noise flooring concentrates signal from a large candidate set into hundreds of critical revenue anomalies (e.g. on the order of **~92k → ~388** alerts) with segment-level contribution mapping.

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

1. **Detect** — seasonality-aware, noise-floored Z-score alerts from `alerts_live` (daily or hourly wall)
2. **Attribute** — multi-dimensional contributors + plain-language `alert_observations` computed in ClickHouse
3. **Investigate** — baseline, metric tree, segments, seasonality, counterfactual (Go engine)
4. **Narrate** — LLM explains engine evidence only
5. **Export** — investigation bundle with trace and evidence hash

## Core techniques (ClickHouse)

| Technique | Summary |
|-----------|---------|
| Seasonality baseline | Same hour / day-of-week over prior ~4 weeks |
| Noise-floored Z-score | e.g. `greatest(stddev, 0.05)` so penny moves do not explode |
| Multi-dim RCA | Concurrent contribution across geo, OS, format, content, tier, campaign |

Details and SQL patterns: [docs/pipeline.md](docs/pipeline.md).

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
| [docs/pipeline.md](docs/pipeline.md) | Native ClickHouse cascade |
| [docs/setup.md](docs/setup.md) | Local setup |
| [docs/data-model.md](docs/data-model.md) | Schema & engines |
| [docs/api-reference.md](docs/api-reference.md) | HTTP APIs |
| [docs/product-guide.md](docs/product-guide.md) | UI behavior |

## Export an investigation

UI: Investigation → **Export**

```bash
node scripts/export-investigation.mjs --list
node scripts/export-investigation.mjs --alertId=<UUID> --out=./exports
```
