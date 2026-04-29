import { useNavigate } from 'react-router-dom'
import { useNamespacesOverview, type NamespaceHealth, type NamespaceStatus } from '../hooks/useNamespacesOverview'
import ErrorBanner from '../components/ErrorBanner'

// ─── status badge ─────────────────────────────────────────────────────────────

const STATUS_META: Record<NamespaceStatus, { label: string; cls: string; dot: string }> = {
  active:   { label: 'Active',   cls: 'bg-green-100 text-green-700 border-green-200',  dot: 'bg-green-500' },
  idle:     { label: 'Idle',     cls: 'bg-blue-100 text-blue-700 border-blue-200',     dot: 'bg-blue-400' },
  degraded: { label: 'Degraded', cls: 'bg-red-100 text-red-700 border-red-200',        dot: 'bg-red-500' },
  cold:     { label: 'Cold',     cls: 'bg-yellow-100 text-yellow-700 border-yellow-200', dot: 'bg-yellow-400' },
}

function StatusBadge({ status }: { status: NamespaceStatus }) {
  const m = STATUS_META[status]
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full border text-xs font-semibold ${m.cls}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${m.dot}`} />
      {m.label}
    </span>
  )
}

// ─── last run summary ────────────────────────────────────────────────────────

function LastRunSummary({ health }: { health: NamespaceHealth }) {
  const run = health.last_run
  if (!run) {
    return <span className="text-gray-400 text-xs">No runs yet</span>
  }
  const when = new Date(run.started_at).toLocaleString()
  const dur = run.duration_ms != null ? `${run.duration_ms} ms` : '—'
  return (
    <span className="text-xs text-gray-500">
      {when} · {dur}
      {!run.success && run.error_message && (
        <span className="ml-1 text-red-500" title={run.error_message}>⚠</span>
      )}
    </span>
  )
}

// ─── namespace card ───────────────────────────────────────────────────────────

function NamespaceCard({ health, onEdit }: { health: NamespaceHealth; onEdit: () => void }) {
  const { config: ns, status, active_events_24h } = health
  const border = status === 'degraded'
    ? 'border-red-200'
    : status === 'active'
      ? 'border-green-200'
      : 'border-gray-200'

  return (
    <div className={`bg-white border ${border} rounded-lg p-4 flex flex-col gap-3`}>
      {/* header */}
      <div className="flex justify-between items-start gap-2">
        <code className="font-mono text-sm font-semibold text-gray-800 break-all">{ns.namespace}</code>
        <StatusBadge status={status} />
      </div>

      {/* metrics row */}
      <div className="grid grid-cols-3 gap-2 text-center">
        <Metric label="Events 24 h" value={active_events_24h.toLocaleString()} />
        <Metric label="Strategy" value={ns.dense_strategy || '—'} />
        <Metric label="Max results" value={String(ns.max_results)} />
      </div>

      {/* last run */}
      <div className="border-t border-gray-100 pt-2">
        <p className="text-xs text-gray-400 mb-0.5">Last batch run</p>
        <LastRunSummary health={health} />
        {status === 'degraded' && health.last_run?.error_message && (
          <details className="mt-1">
            <summary className="text-xs text-red-500 cursor-pointer">show error</summary>
            <pre className="mt-1 text-xs text-red-700 whitespace-pre-wrap leading-tight">
              {health.last_run.error_message}
            </pre>
          </details>
        )}
      </div>

      {/* actions */}
      <div className="flex gap-2 pt-1">
        <button
          onClick={onEdit}
          className="flex-1 bg-transparent border border-gray-300 rounded cursor-pointer px-3 py-1.5 text-sm hover:bg-gray-50"
        >
          Edit config
        </button>
        <a
          href={`/namespaces/${ns.namespace}`}
          onClick={e => { e.preventDefault(); onEdit() }}
          className="flex-1 text-center bg-transparent border border-gray-300 rounded cursor-pointer px-3 py-1.5 text-sm hover:bg-gray-50 text-gray-700 no-underline"
        >
          Vector stats
        </a>
      </div>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-xs text-gray-400 mb-0.5">{label}</p>
      <p className="text-sm font-semibold text-gray-700 truncate">{value}</p>
    </div>
  )
}

// ─── summary bar ──────────────────────────────────────────────────────────────

function SummaryBar({ namespaces }: { namespaces: NamespaceHealth[] }) {
  const counts = { active: 0, idle: 0, degraded: 0, cold: 0 }
  for (const n of namespaces) counts[n.status]++

  return (
    <div className="flex gap-4 mb-4 flex-wrap">
      {(Object.entries(counts) as [NamespaceStatus, number][]).map(([status, count]) => {
        const m = STATUS_META[status]
        return (
          <div key={status} className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg border ${m.cls}`}>
            <span className={`w-2 h-2 rounded-full ${m.dot}`} />
            <span className="text-sm font-semibold">{count}</span>
            <span className="text-xs">{m.label}</span>
          </div>
        )
      })}
    </div>
  )
}

// ─── page ────────────────────────────────────────────────────────────────────

export default function NamespacesPage() {
  const { data, error, isLoading } = useNamespacesOverview()
  const navigate = useNavigate()

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h2 className="m-0 text-xl font-semibold text-gray-800">Namespaces</h2>
        <button
          onClick={() => navigate('/namespaces/new')}
          className="px-4 py-2 bg-blue-600 text-white border-none rounded cursor-pointer text-sm hover:bg-blue-700"
        >
          + Create Namespace
        </button>
      </div>

      {error && <ErrorBanner message="Failed to load namespaces." />}
      {isLoading && <p className="text-gray-400">Loading…</p>}

      {data && data.namespaces.length === 0 && (
        <div className="bg-white border border-gray-200 rounded-lg p-8 text-center text-gray-400">
          No namespaces yet — create one to get started.
        </div>
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
