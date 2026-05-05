import { useActiveNamespace } from '../context/NamespaceContext'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import { useQdrantStats } from '../hooks/useQdrantStats'
import { useBatchRuns } from '../hooks/useBatchRuns'
import { PageHeader, Panel, Table, Thead, Th, Tbody, Tr, Td } from '../components/ui'
import StatusBadge from './namespaces/StatusBadge'
import RunNowButton from './namespaces/RunNowButton'
import type { BatchRunLog } from '../types'

function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const mins = Math.floor(diff / 60_000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

function fmt(n: number | null | undefined): string {
  if (n == null) return '—'
  return n.toLocaleString()
}

function StatTile({ label, value, sub, subClass }: {
  label: string
  value: string
  sub?: string
  subClass?: string
}) {
  return (
    <div className="bg-surface border border-default rounded px-5 py-4">
      <p className="text-xs text-muted mb-1 m-0">{label}</p>
      <p className="text-2xl font-semibold text-primary tabular-nums m-0 leading-tight">{value}</p>
      {sub && <p className={`text-xs mt-1 m-0 ${subClass ?? 'text-muted'}`}>{sub}</p>}
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between py-2.5 border-b border-default last:border-0">
      <span className="text-sm text-muted">{label}</span>
      <span className="text-sm font-medium text-primary font-mono tabular-nums">{value}</span>
    </div>
  )
}

function RunStatusCell({ run }: { run: BatchRunLog }) {
  if (run.success) return <span className="text-success text-sm font-medium">✓ OK</span>
  if (run.completed_at) return <span className="text-danger text-sm font-medium">✗ Failed</span>
  return <span className="text-accent text-sm font-medium">Running</span>
}

export default function NamespaceOverviewPage() {
  const { namespace } = useActiveNamespace()
  const { data: overview } = useNamespacesOverview()
  const { data: qdrantData } = useQdrantStats(namespace ?? '')
  const { data: runsData } = useBatchRuns(namespace || undefined)

  if (!namespace) return null

  const nsHealth = overview?.namespaces.find(n => n.config.namespace === namespace)
  const config = nsHealth?.config
  const lastRun = nsHealth?.last_run
  const recentRuns = runsData?.runs.slice(0, 5) ?? []

  const coll = (key: string) => qdrantData?.collections[`${namespace}_${key}`]
  const subjects      = coll('subjects')
  const objects       = coll('objects')
  const subjectsDense = coll('subjects_dense')
  const objectsDense  = coll('objects_dense')

  const collRow = (label: string, stat: typeof subjects) => (
    <div className="flex items-center justify-between py-2.5 border-b border-default last:border-0">
      <span className="text-sm text-muted">{label}</span>
      {qdrantData
        ? stat?.exists
          ? <span className="text-sm font-medium text-primary tabular-nums font-mono">{fmt(stat.points_count)} pts</span>
          : <span className="text-xs text-muted italic">not created</span>
        : <span className="text-xs text-muted">—</span>
      }
    </div>
  )

  return (
    <div>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            {namespace}
            {nsHealth && <StatusBadge status={nsHealth.status} />}
          </span>
        }
        actions={<RunNowButton ns={namespace} />}
      />

      {/* Stat tiles */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <StatTile
          label="Events (24h)"
          value={fmt(nsHealth?.active_events_24h)}
        />
        <StatTile
          label="Subjects"
          value={fmt(subjects?.points_count)}
          sub="sparse vectors"
        />
        <StatTile
          label="Objects"
          value={fmt(objects?.points_count)}
          sub="sparse vectors"
        />
        <StatTile
          label="Last Run"
          value={lastRun ? relativeTime(lastRun.started_at) : '—'}
          sub={lastRun
            ? lastRun.success ? 'succeeded' : 'failed'
            : 'no runs yet'
          }
          subClass={lastRun
            ? lastRun.success ? 'text-success' : 'text-danger'
            : 'text-muted'
          }
        />
      </div>

      {/* Panels */}
      <div className="grid grid-cols-2 gap-4 mb-4">
        <Panel title="Vector Collections">
          {collRow('Subjects (sparse)', subjects)}
          {collRow('Objects (sparse)', objects)}
          {collRow('Subjects (dense)', subjectsDense)}
          {collRow('Objects (dense)', objectsDense)}
        </Panel>

        <Panel title="Configuration">
          {config ? (
            <>
              <Row label="Max results"       value={String(config.max_results)} />
              <Row label="Sparse weight (α)" value={String(config.alpha)} />
              <Row label="Dense strategy"    value={config.dense_strategy} />
              <Row label="Time decay (λ)"    value={String(config.lambda)} />
              <Row label="Freshness (γ)"     value={String(config.gamma)} />
              <Row label="Seen window"       value={`${config.seen_items_days}d`} />
              <Row label="Trending window"   value={`${config.trending_window}h`} />
            </>
          ) : (
            <p className="text-sm text-muted m-0">Loading…</p>
          )}
        </Panel>
      </div>

      {/* Recent runs */}
      {recentRuns.length > 0 && (
        <Panel title="Recent Batch Runs">
          <Table>
            <Thead>
              <Th>Started</Th>
              <Th>Duration</Th>
              <Th>Subjects</Th>
              <Th>Status</Th>
            </Thead>
            <Tbody>
              {recentRuns.map(run => (
                <Tr key={run.id}>
                  <Td mono>{new Date(run.started_at).toLocaleString()}</Td>
                  <Td muted mono>{run.duration_ms != null ? `${run.duration_ms} ms` : '—'}</Td>
                  <Td muted mono>{fmt(run.subjects_processed)}</Td>
                  <Td><RunStatusCell run={run} /></Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </Panel>
      )}
    </div>
  )
}
