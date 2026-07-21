package batchrun

// TriggerSource enumerates every value the batch_run_logs.trigger_source
// column is allowed to hold. The DB CHECK constraint (migration 012, pre-prod
// hardening) mirrors
// these literals — adding a value here must always come with a migration
// that widens the constraint.
type TriggerSource string

const (
	// TriggerCron is the scheduled batch run produced by cmd/cron every
	// CODOHUE_BATCH_INTERVAL_MINUTES tick.
	TriggerCron TriggerSource = "cron"

	// TriggerManual is the synchronous CF batch produced when an operator
	// clicks "Run batch now" on the admin Overview. Despite the name the
	// runtime path is identical to TriggerCron — the column lets the admin
	// UI distinguish operator action from cron pressure.
	TriggerManual TriggerSource = "manual"

	// TriggerReembed is the catalog re-embed orchestration row written by
	// admin.TriggerReEmbed. The row stays open (completed_at IS NULL) until
	// the embedder watcher closes it; phase columns are intentionally null.
	TriggerReembed TriggerSource = "admin_reembed"
)

// String satisfies fmt.Stringer for ergonomic logging and SQL parameter
// passing — pgx accepts string-kind values directly.
func (t TriggerSource) String() string { return string(t) }

// OperatorCancelledMessage is the canonical batch_run_logs.error_message
// written by cron when it observes cancel_requested. Lives in core/batchrun
// so the admin handler (which filters / labels cancelled rows) and the
// compute job (which writes the row) reference one literal — peer-domain
// imports are forbidden so a shared constant is the only drift-proof option.
const OperatorCancelledMessage = "operator_cancelled"
