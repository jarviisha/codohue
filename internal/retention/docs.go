// Package retention provides a periodic prune job that keeps observability
// tables bounded under steady-state operation:
//
//   - batch_run_logs grows by one row per namespace per cron tick. With
//     CODOHUE_BATCH_INTERVAL_MINUTES=5 and 10 namespaces that's roughly
//     2,880 rows/day — unbounded growth without retention.
//   - catalog_backlog_samples grows by one row per namespace per 30-second
//     sampler tick (skip-on-unchanged trims this, but the worst case still
//     adds rows at a steady rate).
//
// The Job runs in cmd/cron alongside the compute job and is configured via
// CODOHUE_BATCH_RUN_RETENTION_DAYS / CODOHUE_BACKLOG_SAMPLES_RETENTION_DAYS
// / CODOHUE_RETENTION_INTERVAL.
package retention
