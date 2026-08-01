-- InsightIQ ClickHouse view layer (reference)
-- Database: insightiq
-- UI + MCP should query these tables, not ad_events_raw.

-- Raw fact (ingest only)
-- ad_events_raw → mv_hourly → agg_hourly → mv_metric_hourly_snapshot → metric_hourly_snapshot
-- baseline_hourly + metric_hourly_snapshot → alerts_live
-- alerts_live → alert_dimension_contributors + alert_observations

-- Dashboard: alert wall
-- SELECT alert_id, advertiser_id, bucket, actual, expected, zscore
-- FROM insightiq.alerts_live ORDER BY bucket DESC;

-- Alert details
-- SELECT title, detail, impact FROM insightiq.alert_observations
-- WHERE alert_id = {alert_id} ORDER BY observation_order;
-- SELECT dimension, dimension_value, delta, contribution
-- FROM insightiq.alert_dimension_contributors
-- WHERE alert_id = {alert_id} ORDER BY abs(delta) DESC;

-- Chat / MCP: narrate observations only
-- SELECT detail FROM insightiq.alert_observations WHERE alert_id = {alert_id};
