package batchrun

import "errors"

// ErrRunInProgress is returned when the per-namespace cross-process compute
// lock is already held — another run (cron tick, manual trigger, or a
// namespace wipe) owns the namespace right now. Lives in core/batchrun so
// the admin plane can map it to 409 without importing internal/compute.
var ErrRunInProgress = errors.New("batch run already in progress")
