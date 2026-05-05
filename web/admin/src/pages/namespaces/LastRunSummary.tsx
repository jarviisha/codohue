import type { NamespaceHealth } from '../../types'

export default function LastRunSummary({ health }: { health: NamespaceHealth }) {
  const run = health.last_run
  if (!run) return <span className="text-xs text-muted">No runs yet</span>
  const when = new Date(run.started_at).toLocaleString()
  const dur = run.duration_ms != null ? `${run.duration_ms} ms` : '—'
  return (
    <span className="text-xs text-muted tabular-nums">
      {when} · {dur}
      {!run.success && run.error_message && (
        <span className="ml-1 text-danger" title={run.error_message}>!</span>
      )}
    </span>
  )
}
