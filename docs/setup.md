# Setup & run

## Prerequisites

- Node.js 20+
- Go 1.22+
- ClickHouse Cloud database `insightiq` loaded with the hackathon dataset
- (Optional) Gemini API key, Langfuse project, LibreChat

## Environment files

### `apps/engine/.env`

Copy from `.env.example`:

```bash
ENGINE_PORT=4100
CLICKHOUSE_HOST=<your-cloud-host>
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

# Optional Langfuse (JP cloud example)
LANGFUSE_SECRET_KEY=
LANGFUSE_PUBLIC_KEY=
LANGFUSE_BASE_URL=https://jp.cloud.langfuse.com
```

### `apps/web/.env`

```bash
VITE_API_URL=http://localhost:4000
VITE_LIBRECHAT_URL=http://localhost:3080
```

## Start services (3 terminals)

```bash
# 1) Engine
cd apps/engine
go build -o bin/engine .
./bin/engine
# → http://localhost:4100

# 2) API
cd apps/api
npm install
npm run dev   # or: node src/index.js
# → http://localhost:4000

# 3) Web
cd apps/web
npm install
npm run dev
# → http://localhost:5173
```

### Health checks

```bash
curl -s http://127.0.0.1:4100/health
curl -s http://127.0.0.1:4000/health
```

Engine health includes `alerts` count and `database`. API health includes `engine`, `gemini`, and `langfuse`.

## LibreChat (optional)

Config under `infra/librechat/`. Point LibreChat’s custom endpoint at:

`http://host.docker.internal:4000/v1` (or your machine’s API URL)

Model id: `insightiq-rca`.

Web “Open in LibreChat” uses `VITE_LIBRECHAT_URL`.

## Langfuse (optional)

With keys set in `apps/api/.env`, each chat turn emits:

- `handle-chat-completion`
  - `retrieve-dashboard-evidence` *or* investigation resolve
  - `narrate-with-gemini`

Sessions are keyed by `sessionId` from the web client.

## Unseen-incident export CLI

```bash
# List live alert UUIDs
node scripts/export-unseen.mjs --list

# Export by alert
node scripts/export-unseen.mjs --alertId=<UUID> --out=./unseen-submission

# Export by investigation
node scripts/export-unseen.mjs --investigationId=inv-<UUID>
```

Also available in the Investigation UI: **Export unseen bundle**.

## Common issues

| Symptom | Likely cause |
|---------|----------------|
| `/api/alerts` empty | `alerts_live` empty, or engine down |
| Chat uses wrong dates | Prefer ISO or natural dates (`21 June 2026`); otherwise latest snapshot window |
| Dashboard meta slow | Fixed: date bounds query `agg_hourly`, not the VIEW |
| Engine exit 137 | Process was SIGKILL’d — restart `./bin/engine` |
| Mock-looking alerts | Live-only mode: no mock fallbacks; restart API after pulls |
