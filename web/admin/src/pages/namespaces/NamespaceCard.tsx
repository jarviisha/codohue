import { useTriggerBatch } from '../../hooks/useTriggerBatch'
import ErrorBanner from '../../components/ErrorBanner'
import type { NamespaceHealth } from '../../types'
import { Badge, Button, CodeBadge, MetricTile } from '../../components/ui'
import LastRunSummary from './LastRunSummary'
import RunNowButton from './RunNowButton'
import StatusBadge from './StatusBadge'
import { formatCount } from '../../utils/format'

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
    <div className={`flex flex-col gap-4 rounded-lg border bg-surface p-5 duration-150 ${borderClass}`}>
      <div className="flex justify-between items-start gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <CodeBadge className="text-sm text-primary break-all">{ns.namespace}</CodeBadge>
          {isActive && (
            <Badge tone="accent" className="shrink-0">
              active
            </Badge>
          )}
        </div>
        <StatusBadge status={status} />
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <MetricTile label="Events 24h" value={formatCount(active_events_24h)} valueClassName="text-lg" />
        <MetricTile label="Strategy" value={ns.dense_strategy || '—'} valueClassName="text-sm" />
        <MetricTile label="Max results" value={String(ns.max_results)} valueClassName="text-lg" />
      </div>

      <div className="pt-3">
        <p className="text-xs text-muted mb-1 m-0">Last batch run</p>
        <LastRunSummary health={health} />
        {degraded && health.last_run?.error_message && (
          <details className="mt-1.5">
            <summary className="text-xs cursor-pointer font-medium text-danger">show error</summary>
            <pre className="mt-1 text-xs whitespace-pre-wrap leading-tight text-danger">
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
            Active
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
