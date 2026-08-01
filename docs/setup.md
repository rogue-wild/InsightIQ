# Setup & run

## Prerequisites

- Node.js 20+
- Go 1.22+
- ClickHouse Cloud (or compatible) database `insightiq` with the InsightIQ schema and data
- Optional: Gemini API key, Langfuse, LibreChat

## Environment files

### `apps/engine/.env`

```bash
ENGINE_PORT=4100
CLICKHOUSE_HOST=<host>
CLICKHOUSE_PORT=8443
CLICKHOUSE_USER=default
CLICKHOUSE_PASSWORD=<secret>
CLICKHOUSE_DATABASE=insightiq
CLICKHOUSE_SECURE=true
CLICKHOUSE_LOG_QUERIES=true
```

### `apps/api/.env`

```bash
PORT=4000
ENGINE_URL=http://127.0.0.1:4100
GEMINI_API_KEY=<optional>
GEMINI_MODEL=gemini-flash-lite-latest

LANGFUSE_SECRET_KEY=
LANGFUSE_PUBLIC_KEY=
LANGFUSE_BASE_URL=https://jp.cloud.langfuse.com
```

### `apps/web/.env`

```bash
VITE_API_URL=http://localhost:4000
VITE_LIBRECHAT_URL=http://localhost:3080
```

## Start services

```bash
# Engine
cd apps/engine && go build -o bin/engine . && ./bin/engine

# API
cd apps/api && npm install && npm run dev

# Web
cd apps/web && npm install && npm run dev
```

| Service | URL |
|---------|-----|
| Web | http://localhost:5173 |
| API | http://localhost:4000 |
| Engine | http://localhost:4100 |

```bash
curl -s http://127.0.0.1:4100/health
curl -s http://127.0.0.1:4000/health
```

## LibreChat (optional)

Config under `infra/librechat/`. Point the custom endpoint at the API `/v1` base. Model id: `insightiq-rca`.

## Langfuse (optional)

With keys set, chat turns emit `handle-chat-completion` → retrieve/investigate → `narrate-with-gemini`.

## Investigation export CLI

```bash
node scripts/export-investigation.mjs --list
node scripts/export-investigation.mjs --alertId=<UUID> --out=./exports
node scripts/export-investigation.mjs --investigationId=inv-<UUID>
```

Also available in the Investigation UI as **Export** .

## Verify the ClickHouse cascade

With the `insightiq` schema loaded, sanity-check observations and alert volume:

```sql
SELECT title, detail, impact
FROM insightiq.alert_observations
ORDER BY abs(impact) DESC
LIMIT 5;

SELECT count() FROM insightiq.alerts_live WHERE abs(zscore) > 3;
```

Full cascade, techniques, and glossary: [pipeline.md](./pipeline.md).

## Troubleshooting

| Symptom | Likely cause |
|---------|----------------|
| Empty `/api/alerts` | No rows in `alerts_live`, or engine down |
| Chat wrong date window | Pass an explicit date, or ensure snapshot `dataRange` is populated |
| Dashboard meta slow | Engine should bound dates via `agg_hourly` |
| API 502 on alerts | Engine not reachable at `ENGINE_URL` |
