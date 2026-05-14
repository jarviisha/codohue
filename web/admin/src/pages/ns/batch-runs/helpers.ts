import type { StatusState } from '@/components/ui'
import type { BatchRunLog } from '@/services/batchRuns'

// runStatusToken collapses (completed_at, success) into the visual state of
// a row: running → 'run', success → 'ok', failure → 'fail'.
export function runStatusToken(row: BatchRunLog): StatusState {
  if (!row.completed_at) return 'run'
  return row.success ? 'ok' : 'fail'
}

export function runStatusLabel(row: BatchRunLog): string {
  if (!row.completed_at) return 'running'
  return row.success ? 'ok' : 'failed'
}

// phaseToken maps a tri-state phase column (null = skipped, true = ok,
// false = failed) to the matching StatusToken state.
export function phaseToken(ok: boolean | null | undefined): StatusState {
  if (ok === null || ok === undefined) return 'idle'
  return ok ? 'ok' : 'fail'
}

// formatPhaseDuration returns "—" for null/undefined or "Xs"/"Yms" for set values.
export function formatPhaseDuration(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return '—'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}
