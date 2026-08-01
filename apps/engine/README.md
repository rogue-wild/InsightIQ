# InsightIQ Go investigation engine

Reads ClickHouse `insightiq` pre-computed tables:

`alerts_live` → `alert_dimension_contributors` / `alert_observations` → evidence JSON  
(+ `metric_hourly_snapshot` for revenue-identity decomposition)

Node/Gemini only narrates this evidence.

## Config

Copy `.env.example` → `.env` (gitignored) with ClickHouse Cloud settings.
Set `CLICKHOUSE_DATABASE=insightiq`.

## Run

```bash
go run .
# or
go build -o bin/engine . && ./bin/engine
```

Listens on `:4100` by default.

## Endpoints

- `GET /health` — ping + `alerts_live` count
- `POST /investigate` — body `{ alertId }` (UUID from `alerts_live`) or window fields
- `GET /investigations/:id` — cached / rebuild (`inv-{uuid}`)
- `GET /alerts` — top `alerts_live` rows (prefers those with observations)

## Notes

Uses ClickHouse **HTTPS HTTP** interface (Cloud `:8443`), not native protocol.
Never scans `ad_events_raw` for UI or chat paths.
