import { useActiveNamespace } from '../context/useActiveNamespace'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import { useQdrant } from '../hooks/useQdrant'
import { useBatchRuns } from '../hooks/useBatchRuns'
import { Badge, KeyValueList, KeyValueRow, LoadingState, MetricTile, PageHeader, PageShell, Panel, Table, Thead, Th, Tbody, Tr, Td } from '../components/ui'
import StatusBadge from './namespaces/StatusBadge'
import RunNowButton from './namespaces/RunNowButton'
import type { BatchRunLog } from '../types'
import { formatCount, formatDateTime, formatDurationMs, formatRelativeTime } from '../utils/format'

function RunStatusCell({ run }: { run: BatchRunLog }) {
  if (run.success) return <Badge tone="success" dot>OK</Badge>
  if (run.completed_at) return <Badge tone="danger" dot>Failed</Badge>
  return <Badge tone="accent" dot>Running</Badge>
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
