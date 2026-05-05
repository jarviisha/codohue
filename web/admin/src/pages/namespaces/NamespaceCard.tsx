import { useTriggerBatch } from '../../hooks/useTriggerBatch'
import ErrorBanner from '../../components/ErrorBanner'
import type { NamespaceHealth } from '../../types'
import { Button, CodeBadge } from '../../components/ui'
import LastRunSummary from './LastRunSummary'
import Metric from './Metric'
import RunNowButton from './RunNowButton'
import StatusBadge from './StatusBadge'

interface Props {
  health: NamespaceHealth
  onEdit: () => void
  onSelect: () => void
  isActive?: boolean
}

export default function NamespaceCard({ health, onEdit, onSelect, isActive }: Props) {
  const { config: ns, status, active_events_24h } = health
  const degraded = status === 'degraded'
  const trigger = useTriggerBatch(ns.namespace)

  let borderClass = 'border-default'
  if (isActive) borderClass = 'border-accent/50'
  else if (degraded) borderClass = 'border-danger/30'

  return (
    <div className={`bg-surface flex flex-col gap-4 p-5 rounded border duration-150 ${borderClass}`}>
      <div className="flex justify-between items-start gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <CodeBadge className="text-sm text-primary break-all">{ns.namespace}</CodeBadge>
          {isActive && (
            <span className="text-[10px] font-semibold uppercase tracking-[0.06em] px-1.5 py-0.5 rounded bg-accent-subtle text-accent border border-accent/20 shrink-0">
              active
            </span>
          )}
        </div>
        <StatusBadge status={status} />
      </div>

      <div className="grid grid-cols-3 gap-3 border border-default rounded p-2">
        <Metric label="Events 24h" value={active_events_24h.toLocaleString()} />
        <Metric label="Strategy" value={ns.dense_strategy || '—'} />
        <Metric label="Max results" value={String(ns.max_results)} />
      </div>

      <div className="pt-3">
        <p className="text-xs text-muted mb-1 m-0">Last batch run</p>
        <LastRunSummary health={health} />
        {degraded && health.last_run?.error_message && (
          <details className="mt-1.5">
            <summary className="text-xs cursor-pointer font-medium text-danger">show error</summary>
            <pre className="mt-1 text-xs whitespace-pre-wrap leading-tight text-danger font-mono">
              {health.last_run.error_message}
            </pre>
          </details>
        )}
        {trigger.error && (
          <ErrorBanner message={trigger.error.message} />
        )}
      </div>

      <div className="flex gap-2">
        {isActive ? (
          <Button size="sm" className="flex-1" disabled>
            ✓ Active
          </Button>
        ) : (
          <Button variant="primary" size="sm" className="flex-1" onClick={onSelect}>
            Select
          </Button>
        )}
        <Button size="sm" onClick={onEdit}>
          Edit
        </Button>
        <RunNowButton ns={ns.namespace} />
      </div>
    </div>
  )
}
