# LibreChat for InsightIQ

Your running LibreChat install: `/Users/geospot/Developer/LibreChat` (port **3080**).

## ClickHouse MCP (wired)

LibreChat talks to a `mcp-clickhouse` Docker service that queries your **ClickHouse Cloud** `insightiq` database.

1. Ensure LibreChat `.env` has:
   - `CLICKHOUSE_HOST`, `CLICKHOUSE_PORT`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`
   - `CLICKHOUSE_DATABASE=insightiq`
   - `CLICKHOUSE_MCP_AUTH_TOKEN=<random secret>`
2. `librechat.yaml` includes `mcpServers.clickhouse-insightiq` (SSE) + InsightIQ custom endpoint
3. Restart:

```bash
cd /Users/geospot/Developer/LibreChat
docker compose up -d
```

4. Open http://localhost:3080 → select MCP **clickhouse-insightiq**
5. Try: `List databases` or `SELECT count() FROM insightiq.alerts_live`

Also keep InsightIQ Node on `:4000` if you use the **InsightIQ** custom chat endpoint.

## Smoke test MCP health

```bash
curl -s http://localhost:8001/health
```

Should return `OK` when MCP can reach ClickHouse Cloud.

## Preferred MCP tables

Use the pre-computed view layer only:

- `alerts_live`
- `alert_observations`
- `alert_dimension_contributors`
- `metric_hourly_snapshot` / `agg_hourly`
- `baseline_hourly`

Do not query `ad_events_raw` from chat.
