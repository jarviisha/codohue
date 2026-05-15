import type { NamespaceHealth } from '../../types'
import { formatDateTime, formatDurationMs } from '../../utils/format'

export default function LastRunSummary({ health }: { health: NamespaceHealth }) {
  const run = health.last_run
  if (!run) return <span className="text-xs text-muted">No runs yet</span>
  const when = formatDateTime(run.started_at)
  const dur = formatDurationMs(run.duration_ms)
  return (
    <span className="text-xs text-muted tabular-nums">
      {when} · {dur}
      {!run.success && run.error_message && (
        <span className="ml-1 text-danger" title={run.error_message}>!</span>
      )}
    </span>
  )
}
