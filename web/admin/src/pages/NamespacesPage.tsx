import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import { useTriggerBatch } from '../hooks/useTriggerBatch'
import ErrorBanner from '../components/ErrorBanner'
import type { NamespaceHealth, NamespaceStatus } from '../types'
import { Button, CodeBadge, EmptyState, PageHeader } from '../components/ui'

const STATUS_META: Record<NamespaceStatus, { label: string; wrap: string; dot: string; text: string }> = {
  active:   { label: 'Active',   wrap: 'bg-success-bg border border-success/30',  dot: 'bg-success', text: 'text-success' },
  idle:     { label: 'Idle',     wrap: 'bg-accent-subtle border border-accent/20', dot: 'bg-accent',  text: 'text-accent' },
  degraded: { label: 'Degraded', wrap: 'bg-danger-bg border border-danger/25',     dot: 'bg-danger',  text: 'text-danger' },
  cold:     { label: 'Cold',     wrap: 'bg-warning-bg border border-warning/30',   dot: 'bg-warning', text: 'text-warning' },
}

function StatusBadge({ status }: { status: NamespaceStatus }) {
  const m = STATUS_META[status]
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.04em] rounded-sm ${m.wrap} ${m.text}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${m.dot}`} />
      {m.label}
    </span>
  )
}

function LastRunSummary({ health }: { health: NamespaceHealth }) {
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

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="text-center">
      <p className="text-xs text-muted mb-0.5 m-0">{label}</p>
      <p className="text-sm font-medium text-primary truncate m-0 tabular-nums">{value}</p>
    </div>
  )
}

function RunNowButton({ ns }: { ns: string }) {
  const trigger = useTriggerBatch(ns)
  const [showDone, setShowDone] = useState(false)

  async function handleClick() {
    try {
      await trigger.mutateAsync()
      setShowDone(true)
      setTimeout(() => setShowDone(false), 2000)
    } catch {
      // error shown below
    }
  }

  if (showDone) {
    return (
      <span className="flex-1 text-center py-1.5 px-3 text-sm font-semibold text-success bg-success-bg border border-success/30 rounded-md">
        Done
      </span>
    )
  }

  return (
    <Button
      onClick={handleClick}
      disabled={trigger.isPending}
      size="sm"
      className="flex-1"
    >
      {trigger.isPending ? 'Running…' : 'Run now'}
    </Button>
  )
}

function NamespaceCard({ health, onEdit }: { health: NamespaceHealth; onEdit: () => void }) {
  const { config: ns, status, active_events_24h } = health
  const degraded = status === 'degraded'
  const trigger = useTriggerBatch(ns.namespace)

  return (
    <div className={`bg-surface flex flex-col gap-4 p-5 rounded-lg border transition-shadow duration-150 hover:shadow-floating ${degraded ? 'border-danger/30' : 'border-default'}`}>
      <div className="flex justify-between items-start gap-2">
        <CodeBadge className="text-sm text-primary break-all">{ns.namespace}</CodeBadge>
        <StatusBadge status={status} />
      </div>

      <div className="grid grid-cols-3 gap-3">
        <Metric label="Events 24h" value={active_events_24h.toLocaleString()} />
        <Metric label="Strategy" value={ns.dense_strategy || '—'} />
        <Metric label="Max results" value={String(ns.max_results)} />
      </div>

      <div className="pt-3 border-t border-default">
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
        <Button
          onClick={onEdit}
          size="sm"
          className="flex-1"
        >
          Edit config
        </Button>
        <a
          href={`/namespaces/${ns.namespace}`}
          onClick={e => { e.preventDefault(); onEdit() }}
          className="flex-1 text-center py-1.5 px-3 text-sm font-medium no-underline transition-colors duration-150 bg-transparent border border-default hover:border-strong hover:bg-surface-raised text-primary rounded-md"
        >
          Vector stats
        </a>
        <RunNowButton ns={ns.namespace} />
      </div>
    </div>
  )
}

function SummaryBar({ namespaces }: { namespaces: NamespaceHealth[] }) {
  const counts = { active: 0, idle: 0, degraded: 0, cold: 0 }
  for (const n of namespaces) counts[n.status]++

  return (
    <div className="flex gap-3 mb-6 flex-wrap">
      {(Object.entries(counts) as [NamespaceStatus, number][]).map(([status, count]) => {
        const m = STATUS_META[status]
        return (
          <div
            key={status}
            className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md ${m.wrap} ${m.text}`}
          >
            <span className={`w-2 h-2 rounded-full ${m.dot}`} />
            <span className="tabular-nums font-semibold">{count}</span>
            <span className="text-xs">{m.label}</span>
          </div>
        )
      })}
    </div>
  )
}

export default function NamespacesPage() {
  const { data, error, isLoading } = useNamespacesOverview()
  const navigate = useNavigate()

  return (
    <div>
      <PageHeader
        title="Namespaces"
        actions={(
          <Button variant="primary" onClick={() => navigate('/namespaces/new')}>
            + Create Namespace
          </Button>
        )}
      />

      {error && <ErrorBanner message="Failed to load namespaces." />}
      {isLoading && <p className="text-sm text-muted">Loading…</p>}

      {data && data.namespaces.length === 0 && (
        <EmptyState>
          No namespaces yet — create one to get started.
        </EmptyState>
      )}

      {data && data.namespaces.length > 0 && (
        <>
          <SummaryBar namespaces={data.namespaces} />
          <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))' }}>
            {data.namespaces.map(h => (
              <NamespaceCard
                key={h.config.namespace}
                health={h}
                onEdit={() => navigate(`/namespaces/${h.config.namespace}`)}
              />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
