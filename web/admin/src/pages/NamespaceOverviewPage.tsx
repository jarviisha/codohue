import { Link } from 'react-router-dom'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import { useQdrant } from '../hooks/useQdrant'
import { useBatchRuns } from '../hooks/useBatchRuns'
import { Badge, KeyValueList, KeyValueRow, LoadingState, MetricTile, PageHeader, PageShell, Panel, Table, Thead, Th, Tbody, Tr, Td } from '../components/ui'
import StatusBadge from './namespaces/StatusBadge'
import RunNowButton from './namespaces/RunNowButton'
import type { BatchRunLog } from '../types'
import { formatCount, formatDateTime, formatDurationMs, formatRelativeTime } from '../utils/format'

type StatusTone = 'neutral' | 'accent' | 'success' | 'warning' | 'danger'

function RunStatusCell({ run }: { run: BatchRunLog }) {
  if (run.success) return <Badge tone="success" dot>OK</Badge>
  if (run.completed_at) return <Badge tone="danger" dot>Failed</Badge>
  return <Badge tone="accent" dot>Running</Badge>
}

function StatusRow({
  label,
  detail,
  tone,
  action,
}: {
  label: string
  detail: string
  tone: StatusTone
  action?: { label: string; to: string }
}) {
  return (
    <div className="flex flex-col gap-2 border-b border-default py-3 last:border-b-0 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <Badge tone={tone} dot>
            {label}
          </Badge>
        </div>
        <p className="m-0 mt-1 text-sm text-secondary">{detail}</p>
      </div>
      {action && (
        <Link
          to={action.to}
          className="shrink-0 rounded px-2 py-1 text-sm font-medium text-accent no-underline hover:bg-accent-subtle focus-visible:outline-none focus-visible:shadow-focus"
        >
          {action.label}
        </Link>
      )}
    </div>
  )
}

export default function NamespaceOverviewPage() {
  const { namespace } = useActiveNamespace()
  const { data: overview } = useNamespacesOverview()
  const { data: qdrantData } = useQdrant(namespace ?? '')
  const { data: runsData } = useBatchRuns(namespace || undefined)

  if (!namespace) return null

  const nsHealth = overview?.items.find(n => n.config.namespace === namespace)
  const config = nsHealth?.config
  const lastRun = nsHealth?.last_run
  const recentRuns = runsData?.items.slice(0, 5) ?? []

  const coll = (key: 'subjects' | 'objects' | 'subjects_dense' | 'objects_dense') => qdrantData?.[key]
  const subjects      = coll('subjects')
  const objects       = coll('objects')
  const subjectsDense = coll('subjects_dense')
  const objectsDense  = coll('objects_dense')
  const nsBase = `/namespaces/${encodeURIComponent(namespace)}`
  const sparseReady = Boolean(subjects?.exists && objects?.exists)
  const denseReady = Boolean(subjectsDense?.exists && objectsDense?.exists)
  const eventCount = nsHealth?.active_events_24h ?? 0

  let batchTone: StatusTone = 'warning'
  let batchDetail = 'No batch run has completed for this namespace.'
  if (lastRun && !lastRun.completed_at) {
    batchTone = 'accent'
    batchDetail = `Batch run #${lastRun.id} is currently running.`
  } else if (lastRun?.success) {
    batchTone = 'success'
    batchDetail = `Last run succeeded ${formatRelativeTime(lastRun.started_at)}.`
  } else if (lastRun) {
    batchTone = 'danger'
    batchDetail = `Last run failed ${formatRelativeTime(lastRun.started_at)}.`
  }

  const collRow = (label: string, stat: typeof subjects) => (
    <KeyValueRow
      label={label}
      value={
        qdrantData
          ? stat?.exists
            ? `${formatCount(stat.points_count)} pts`
            : <span className="text-xs text-muted italic">not created</span>
          : <span className="text-xs text-muted">—</span>
      }
    />
  )

  return (
    <PageShell>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            {namespace}
            {nsHealth && <StatusBadge status={nsHealth.status} />}
          </span>
        }
        actions={<RunNowButton ns={namespace} />}
      />

      <Panel title="Operational Status">
        <StatusRow
          label={eventCount > 0 ? 'Events flowing' : 'No recent events'}
          tone={eventCount > 0 ? 'success' : 'warning'}
          detail={
            eventCount > 0
              ? `${formatCount(eventCount)} events received in the last 24 hours.`
              : 'No events were received in the last 24 hours.'
          }
          action={{ label: 'View events', to: `${nsBase}/events` }}
        />
        <StatusRow
          label="Batch recompute"
          tone={batchTone}
          detail={batchDetail}
          action={{ label: 'View runs', to: `${nsBase}/batch-runs` }}
        />
        <StatusRow
          label={sparseReady ? 'Sparse vectors ready' : 'Sparse vectors missing'}
          tone={qdrantData ? sparseReady ? 'success' : 'danger' : 'neutral'}
          detail={
            qdrantData
              ? sparseReady
                ? 'Subject and object sparse collections are available.'
                : 'One or more sparse collections have not been created.'
              : 'Qdrant collection status is still loading.'
          }
        />
        <StatusRow
          label={denseReady ? 'Dense vectors ready' : 'Dense vectors incomplete'}
          tone={qdrantData ? denseReady ? 'success' : 'warning' : 'neutral'}
          detail={
            qdrantData
              ? denseReady
                ? 'Subject and object dense collections are available.'
                : 'Dense collections are missing or not populated yet.'
              : 'Qdrant dense collection status is still loading.'
          }
          action={{ label: 'Catalog', to: `${nsBase}/catalog/items` }}
        />
      </Panel>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricTile
          label="Events (24h)"
          value={formatCount(nsHealth?.active_events_24h)}
        />
        <MetricTile
          label="Subjects"
          value={formatCount(subjects?.points_count)}
          sub="sparse vectors"
        />
        <MetricTile
          label="Objects"
          value={formatCount(objects?.points_count)}
          sub="sparse vectors"
        />
        <MetricTile
          label="Last Run"
          value={lastRun ? formatRelativeTime(lastRun.started_at) : '—'}
          sub={lastRun
            ? lastRun.success ? 'succeeded' : 'failed'
            : 'no runs yet'
          }
          subClassName={lastRun
            ? lastRun.success ? 'text-success' : 'text-danger'
            : 'text-muted'
          }
        />
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Panel title="Vector Collections">
          <KeyValueList>
            {collRow('Subjects (sparse)', subjects)}
            {collRow('Objects (sparse)', objects)}
            {collRow('Subjects (dense)', subjectsDense)}
            {collRow('Objects (dense)', objectsDense)}
          </KeyValueList>
        </Panel>

        <Panel title="Configuration">
          {config ? (
            <KeyValueList>
              <KeyValueRow label="Max results" value={String(config.max_results)} />
              <KeyValueRow label="Sparse weight (α)" value={String(config.alpha)} />
              <KeyValueRow label="Dense strategy" value={config.dense_strategy} />
              <KeyValueRow label="Time decay (λ)" value={String(config.lambda)} />
              <KeyValueRow label="Freshness (γ)" value={String(config.gamma)} />
              <KeyValueRow label="Seen window" value={`${config.seen_items_days}d`} />
              <KeyValueRow label="Trending window" value={`${config.trending_window}h`} />
            </KeyValueList>
          ) : (
            <LoadingState />
          )}
        </Panel>
      </div>

      {/* Recent runs */}
      {recentRuns.length > 0 && (
        <Panel title="Recent Batch Runs" bodyClassName="overflow-x-auto">
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
                  <Td muted mono>{formatDateTime(run.started_at)}</Td>
                  <Td mono>{formatDurationMs(run.duration_ms)}</Td>
                  <Td muted mono>{formatCount(run.subjects_processed)}</Td>
                  <Td><RunStatusCell run={run} /></Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </Panel>
      )}
    </PageShell>
  )
}
