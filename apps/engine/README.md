# InsightIQ Go investigation engine

Reads ClickHouse `insightiq` pre-computed layer:

`ad_events_raw` → MV `mv_hourly` → `agg_hourly` → VIEW `metric_hourly_snapshot`  
`alerts_live` → `alert_dimension_contributors` / `alert_observations` → evidence JSON  
(+ `metric_hourly_snapshot` / `agg_hourly` for decomposition + date bounds)

Node/Gemini only narrates this evidence.

## Config

Copy `.env.example` → `.env` (gitignored) with ClickHouse Cloud settings.
Set `CLICKHOUSE_DATABASE=insightiq`.

Every HTTP request logs the ClickHouse SQL it runs (tagged `req#N METHOD /path`).
Disable with `CLICKHOUSE_LOG_QUERIES=false`.

## Run

```bash
go run .
# or
go build -o bin/engine . && ./bin/engine
```

Listens on `:4100` by default (`ENGINE_PORT`). Tail logs to verify queries:

```bash
./bin/engine 2>&1 | tee /tmp/insightiq-engine.log
```

## Endpoints

- `GET /health` — ping + `alerts_live` count
- `POST /investigate` — body `{ alertId }` (UUID from `alerts_live`) or window fields
- `GET /investigations/:id` — cached / rebuild (`inv-{uuid}`)
- `GET /alerts?granularity=day|hour` — alert wall (default `day` = peak hourly anomaly per advertiser+day; `hour` = native hourly buckets)
- `GET /dashboard/meta`, `POST /dashboard/query`, `GET /dashboard/filters`

## Notes

Uses ClickHouse **HTTPS HTTP** interface (Cloud `:8443`), not native protocol.
Never scans `ad_events_raw` for UI or chat paths.
