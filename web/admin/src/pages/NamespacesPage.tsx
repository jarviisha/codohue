import { useNavigate } from 'react-router-dom'
import { useNamespacesOverview, type NamespaceHealth, type NamespaceStatus } from '../hooks/useNamespacesOverview'
import ErrorBanner from '../components/ErrorBanner'

const STATUS_META: Record<NamespaceStatus, { label: string; dot: string; color: string; bg: string; border: string }> = {
  active:   { label: 'Active',   dot: '#15be53', color: '#108c3d', bg: 'rgba(21,190,83,0.08)',  border: 'rgba(21,190,83,0.3)' },
  idle:     { label: 'Idle',     dot: '#533afd', color: '#533afd', bg: 'rgba(83,58,253,0.06)',  border: 'rgba(83,58,253,0.2)' },
  degraded: { label: 'Degraded', dot: '#ea2261', color: '#ea2261', bg: 'rgba(234,34,97,0.06)',  border: 'rgba(234,34,97,0.2)' },
  cold:     { label: 'Cold',     dot: '#f59e0b', color: '#92400e', bg: 'rgba(245,158,11,0.07)', border: 'rgba(245,158,11,0.25)' },
}

function StatusBadge({ status }: { status: NamespaceStatus }) {
  const m = STATUS_META[status]
  return (
    <span
      className="inline-flex items-center gap-1.5 px-2 py-0.5 text-xs font-normal"
      style={{ background: m.bg, border: `1px solid ${m.border}`, borderRadius: '4px', color: m.color }}
    >
      <span className="w-1.5 h-1.5 rounded-full" style={{ background: m.dot }} />
      {m.label}
    </span>
  )
}

function LastRunSummary({ health }: { health: NamespaceHealth }) {
  const run = health.last_run
  if (!run) return <span className="text-xs text-[#64748d] font-light">No runs yet</span>
  const when = new Date(run.started_at).toLocaleString()
  const dur = run.duration_ms != null ? `${run.duration_ms} ms` : '—'
  return (
    <span className="text-xs text-[#64748d] font-light tabular-nums">
      {when} · {dur}
      {!run.success && run.error_message && (
        <span className="ml-1" style={{ color: '#ea2261' }} title={run.error_message}>⚠</span>
      )}
    </span>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="text-center">
      <p className="text-xs text-[#64748d] font-light mb-0.5 m-0">{label}</p>
      <p className="text-sm font-normal text-[#061b31] truncate m-0 tabular-nums">{value}</p>
    </div>
  )
}

function NamespaceCard({ health, onEdit }: { health: NamespaceHealth; onEdit: () => void }) {
  const { config: ns, status, active_events_24h } = health
  const degraded = status === 'degraded'

  return (
    <div
      className="bg-white flex flex-col gap-4 p-5 transition-shadow"
      style={{
        border: degraded ? '1px solid rgba(234,34,97,0.3)' : '1px solid #e5edf5',
        borderRadius: '6px',
        boxShadow: 'rgba(23,23,23,0.06) 0px 3px 6px',
      }}
      onMouseEnter={e => {
        (e.currentTarget as HTMLElement).style.boxShadow = 'rgba(50,50,93,0.25) 0px 30px 45px -30px, rgba(0,0,0,0.1) 0px 18px 36px -18px'
      }}
      onMouseLeave={e => {
        (e.currentTarget as HTMLElement).style.boxShadow = 'rgba(23,23,23,0.06) 0px 3px 6px'
      }}
    >
      <div className="flex justify-between items-start gap-2">
        <code
          className="text-sm text-[#061b31] break-all"
          style={{ fontFamily: "'Source Code Pro', monospace", fontWeight: 500 }}
        >
          {ns.namespace}
        </code>
        <StatusBadge status={status} />
      </div>

      <div className="grid grid-cols-3 gap-3">
        <Metric label="Events 24 h" value={active_events_24h.toLocaleString()} />
        <Metric label="Strategy" value={ns.dense_strategy || '—'} />
        <Metric label="Max results" value={String(ns.max_results)} />
      </div>

      <div className="pt-3" style={{ borderTop: '1px solid #e5edf5' }}>
        <p className="text-xs text-[#64748d] font-light mb-1 m-0">Last batch run</p>
        <LastRunSummary health={health} />
        {degraded && health.last_run?.error_message && (
          <details className="mt-1.5">
            <summary className="text-xs cursor-pointer font-normal" style={{ color: '#ea2261' }}>show error</summary>
            <pre
              className="mt-1 text-xs whitespace-pre-wrap leading-tight"
              style={{ color: '#ea2261', fontFamily: "'Source Code Pro', monospace" }}
            >
              {health.last_run.error_message}
            </pre>
          </details>
        )}
      </div>

      <div className="flex gap-2">
        <button
          onClick={onEdit}
          className="flex-1 py-1.5 px-3 text-sm font-normal cursor-pointer transition-colors"
          style={{ background: 'transparent', border: '1px solid #b9b9f9', borderRadius: '4px', color: '#533afd' }}
          onMouseEnter={e => { (e.currentTarget as HTMLElement).style.background = 'rgba(83,58,253,0.05)' }}
          onMouseLeave={e => { (e.currentTarget as HTMLElement).style.background = 'transparent' }}
        >
          Edit config
        </button>
        <a
          href={`/namespaces/${ns.namespace}`}
          onClick={e => { e.preventDefault(); onEdit() }}
          className="flex-1 text-center py-1.5 px-3 text-sm font-normal no-underline transition-colors"
          style={{ background: 'transparent', border: '1px solid #e5edf5', borderRadius: '4px', color: '#273951', display: 'block' }}
          onMouseEnter={e => { const el = e.currentTarget as HTMLElement; el.style.borderColor = '#b9b9f9'; el.style.color = '#533afd' }}
          onMouseLeave={e => { const el = e.currentTarget as HTMLElement; el.style.borderColor = '#e5edf5'; el.style.color = '#273951' }}
        >
          Vector stats
        </a>
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
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm"
            style={{ background: m.bg, border: `1px solid ${m.border}`, borderRadius: '4px', color: m.color }}
          >
            <span className="w-2 h-2 rounded-full" style={{ background: m.dot }} />
            <span className="tabular-nums font-normal">{count}</span>
            <span className="text-xs font-light">{m.label}</span>
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
      <div className="flex justify-between items-center mb-6">
        <h2
          className="font-light text-[#061b31] m-0"
          style={{ fontSize: '26px', letterSpacing: '-0.26px', lineHeight: 1.12 }}
        >
          Namespaces
        </h2>
        <button
          onClick={() => navigate('/namespaces/new')}
          className="px-4 py-2 text-sm font-normal text-white cursor-pointer transition-colors"
          style={{ background: '#533afd', border: 'none', borderRadius: '4px' }}
          onMouseEnter={e => { (e.currentTarget as HTMLElement).style.background = '#4434d4' }}
          onMouseLeave={e => { (e.currentTarget as HTMLElement).style.background = '#533afd' }}
        >
          + Create Namespace
        </button>
      </div>

      {error && <ErrorBanner message="Failed to load namespaces." />}
      {isLoading && <p className="text-sm text-[#64748d] font-light">Loading…</p>}

      {data && data.namespaces.length === 0 && (
        <div
          className="p-10 text-center text-sm text-[#64748d] font-light"
          style={{ border: '1px dashed #d6d9fc', borderRadius: '6px' }}
        >
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
