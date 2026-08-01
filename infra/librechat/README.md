# LibreChat for InsightIQ

Your running LibreChat install: `/Users/geospot/Developer/LibreChat` (port **3080**).

Primary product chat lives in the InsightIQ web app at `/chat` (same branding + favicon).
LibreChat is white-labeled as **InsightIQ** for MCP / full-assistant workflows.

## White-label

Configured via LibreChat `.env` + `docker-compose.override.yml`:

- `APP_TITLE=InsightIQ`
- Custom footer
- Favicon / logo mounts from `infra/librechat/assets/`

## ClickHouse MCP

1. Ensure LibreChat `.env` has ClickHouse Cloud creds and `CLICKHOUSE_DATABASE=insightiq`
2. Restart:

```bash
cd /Users/geospot/Developer/LibreChat
docker compose up -d
```

3. Open http://localhost:3080 → MCP **clickhouse-insightiq**

## Preferred MCP tables

`alerts_live`, `alert_observations`, `alert_dimension_contributors`,
`metric_hourly_snapshot` / `agg_hourly`, `baseline_hourly`

Do not query `ad_events_raw` from chat.
