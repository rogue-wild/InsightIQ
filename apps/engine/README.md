# AdInsight Go investigation engine

Deterministic ClickHouse RCA: baseline → revenue decomposition → dimension drill-down → evidence JSON.

Node/Gemini only narrates this evidence.

## Config

Copy `.env.example` → `.env` (gitignored) with ClickHouse Cloud settings.

## Run

```bash
go run .
# or
go build -o bin/engine . && ./bin/engine
```

Listens on `:4100` by default.

## Endpoints

- `GET /health` — ping + `ad_events` count
- `POST /investigate` — body `{ metric, windowStart, windowEnd, baselineKind, alertId? }`
- `GET /investigations/:id` — cached investigation
- `GET /alerts` — scan recent days for revenue moves ≥ 5%

## Notes

Uses ClickHouse **HTTPS HTTP** interface (Cloud `:8443`), not native protocol.
