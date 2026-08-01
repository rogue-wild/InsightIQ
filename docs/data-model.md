# Data model (ClickHouse `insightiq`)

Reference SQL comments: [`../infra/clickhouse/insightiq_view_layer.sql`](../infra/clickhouse/insightiq_view_layer.sql).

## Pipeline

```
ad_events_raw  (~9M facts)
      │
      ▼  MATERIALIZED VIEW mv_hourly
      │    (joins apps, advertisers, geo_device)
agg_hourly  (SharedSummingMergeTree, ~8.9M)
      │
      ▼  VIEW metric_hourly_snapshot
      │    (re-agg + fill_rate, ctr, ecpm, rpr)
metric_hourly_snapshot
      │
      ├─► baseline_hourly
      │         │
      │         ▼
      └─► alerts_live  (z-score anomalies)
                │
                ├─► alert_dimension_contributors
                └─► alert_observations

alert_rules  (detection policies)
```

## Core tables / views

### `alerts_live`

| Column | Type | Notes |
|--------|------|-------|
| `alert_id` | UUID | Primary key for RCA |
| `advertiser_id` | String | e.g. `adv_0000` |
| `metric` | String | `revenue`, `ecpm`, `ctr`, … |
| `granularity` | String | Currently `hourly` in loaded data |
| `bucket` | DateTime | Hour bucket |
| `actual` / `expected` / `zscore` | Float64 | Anomaly math |
| `created_at` | DateTime | |

Product filter: `abs(zscore) > 3`.

### `alert_dimension_contributors`

Attribution waterfall rows: `dimension`, `dimension_value`, `current_value`, `baseline_value`, `delta`, `contribution`.

### `alert_observations`

Ordered NL notes: `observation_order`, `observation_type`, `title`, `detail`, `impact`.

### `metric_hourly_snapshot` (VIEW)

Columns include dimensions (`region`, `country`, `os_version`, `ad_format`, …) and metrics (`requests`, `fills`, `impressions`, `clicks`, `revenue`, `fill_rate`, `ctr`, `ecpm`, `rpr`).

Used by dashboard queries and investigation baseline / segment ranking.

### `agg_hourly`

Physical rollup. Prefer this for cheap `min(bucket)` / `max(bucket)` date bounds.

### `alert_rules` (live schema)

| Column | Notes |
|--------|-------|
| `rule_id` | UUID |
| `name` | Optional label |
| `metric` | e.g. `revenue` |
| `granularity` | e.g. `hourly` |
| `threshold` | Z-score threshold (e.g. `3`) |
| `min_volume` | Floor before alerting |
| `consecutive_buckets` | Persistence requirement |
| `dimensions` | Array of dimensions to audit |
| `created_at` | |

> Older drafts used `advertiser_id` / `sensitivity` / `enabled`. The **deployed** schema uses `threshold` + `consecutive_buckets` as above.

## Typical loaded volumes (demo)

Approximate after full load:

| Object | Scale |
|--------|-------|
| `ad_events_raw` | ~9M |
| `agg_hourly` | ~8.9M |
| `metric_hourly_snapshot` | ~8.7M (via view) |
| `alerts_live` | hundreds–tens of thousands (varies by load) |
| Snapshot window | ~2026-06-01 → 2026-07-05 |

Contributor / observation coverage may be sparse (e.g. mostly `vertical=entertainment`). Alert category tabs only light up when contributors exist for that dimension family.

## Product queries (patterns)

**Alert wall (hourly):** top `|z|` from `alerts_live`.

**Alert wall (daily):** peak `|z|` per `advertiser_id + metric + toDate(bucket)`, then diversify.

**Contributors / observations:** filter by `alert_id = toUUID(...)`.

**Dashboard:** filter `metric_hourly_snapshot` by time + dimensions; compare prior equal-length window.
