# Data model (ClickHouse `insightiq`)

Reference: [`../infra/clickhouse/insightiq_view_layer.sql`](../infra/clickhouse/insightiq_view_layer.sql).

## Pipeline

```
ad_events_raw
      │
      ▼  MATERIALIZED VIEW mv_hourly
agg_hourly  (SummingMergeTree)
      │
      ▼  VIEW metric_hourly_snapshot
metric_hourly_snapshot
      │
      ├─► baseline_hourly
      │         │
      │         ▼
      └─► alerts_live
                │
                ├─► alert_dimension_contributors
                └─► alert_observations

alert_rules
```

## Objects

### `alerts_live`

| Column | Type |
|--------|------|
| `alert_id` | UUID |
| `advertiser_id` | String |
| `metric` | String |
| `granularity` | String |
| `bucket` | DateTime |
| `actual` / `expected` / `zscore` | Float64 |
| `created_at` | DateTime |

Default product threshold: `abs(zscore) > 3`.

### `alert_dimension_contributors`

`dimension`, `dimension_value`, `current_value`, `baseline_value`, `delta`, `contribution`.

### `alert_observations`

`observation_order`, `observation_type`, `title`, `detail`, `impact`.

### `metric_hourly_snapshot` (VIEW)

Dimensions (`region`, `country`, `os_version`, `ad_format`, …) and metrics (`requests`, `fills`, `impressions`, `clicks`, `revenue`, `fill_rate`, `ctr`, `ecpm`, `rpr`).

### `agg_hourly`

Physical hourly rollup. Used for efficient date-bound queries (`min` / `max` bucket).

### `alert_rules`

| Column | Notes |
|--------|-------|
| `rule_id` | UUID |
| `name` | Optional label |
| `metric` | e.g. `revenue` |
| `granularity` | e.g. `hourly` |
| `threshold` | Z-score threshold |
| `min_volume` | Minimum volume before alerting |
| `consecutive_buckets` | Persistence requirement |
| `dimensions` | Dimensions to audit |
| `created_at` | |

## Query patterns

- **Hourly alert wall:** top `|z|` from `alerts_live`
- **Daily alert wall:** peak `|z|` per advertiser + metric + UTC day
- **Contributors / observations:** by `alert_id`
- **Dashboard:** filter `metric_hourly_snapshot` by time and dimensions; optional compare window
