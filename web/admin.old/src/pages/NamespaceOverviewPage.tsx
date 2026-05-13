import { Link } from 'react-router-dom'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import { useQdrant } from '../hooks/useQdrant'
import { useBatchRuns } from '../hooks/useBatchRuns'
import ErrorBanner from '../components/ErrorBanner'
import { Badge, EmptyState, KeyValueList, KeyValueRow, LoadingState, MetricTile, PageShell, Panel, Table, Thead, Th, Tbody, Tr, Td } from '../components/ui'
import StatusBadge from './namespaces/StatusBadge'
import RunNowButton from './namespaces/RunNowButton'
import type { BatchRunLog } from '../types'
import { formatCount, formatDateTime, formatDurationMs, formatRelativeTime } from '../utils/format'

type StatusTone = 'neutral' | 'accent' | 'success' | 'warning' | 'danger'

function RunStatusCell({ run }: { run: BatchRunLog }) {
  const status = getRunStatus(run)
  return <Badge tone={status.tone} dot>{status.label}</Badge>
}

function getRunStatus(run: BatchRunLog): { label: string; tone: StatusTone; summary: string } {
  if (!run.completed_at) return { label: 'Running', tone: 'accent', summary: 'running' }
  if (run.success) return { label: 'OK', tone: 'success', summary: 'succeeded' }
  return { label: 'Failed', tone: 'danger', summary: 'failed' }
}

function runStatusClass(status: ReturnType<typeof getRunStatus> | null) {
  if (!status) return 'text-muted'
  if (status.tone === 'accent') return 'text-accent'
  if (status.tone === 'success') return 'text-success'
  return 'text-danger'
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof Error) return error.message
  return fallback
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
  const { data: overview, error: overviewError, isLoading: overviewLoading } = useNamespacesOverview()
  const { data: qdrantData, error: qdrantError, isLoading: qdrantLoading } = useQdrant(namespace ?? '')
  const { data: runsData, error: runsError, isLoading: runsLoading } = useBatchRuns(namespace || undefined)

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
  const readyCollections = [subjects, objects, subjectsDense, objectsDense].filter(c => c?.exists).length
  const runStatus = lastRun ? getRunStatus(lastRun) : null
  const isInitialLoading = overviewLoading && !overview

  let batchTone: StatusTone = 'warning'
  let batchDetail = 'No batch run has completed for this namespace.'
  if (lastRun && !lastRun.completed_at) {
    batchTone = 'accent'
    batchDetail = `Batch run #${lastRun.id} is currently running.`
  } else if (lastRun) {
    batchTone = runStatus?.tone ?? 'warning'
    batchDetail = `Last run ${runStatus?.summary ?? 'completed'} ${formatRelativeTime(lastRun.started_at)}.`
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
    <PageShell
      title={
        <span className="flex items-center gap-3">
          {namespace}
          {nsHealth && <StatusBadge status={nsHealth.status} />}
        </span>
      }
      actions={<RunNowButton ns={namespace} />}
    >
      {overviewError && (
        <ErrorBanner message={errorMessage(overviewError, 'Could not load namespace overview.')} />
      )}
      {qdrantError && (
        <ErrorBanner message={errorMessage(qdrantError, 'Could not load Qdrant collection status.')} />
      )}
      {runsError && (
        <ErrorBanner message={errorMessage(runsError, 'Could not load recent batch runs.')} />
      )}

      {isInitialLoading ? (
        <LoadingState label="Loading namespace overview..." />
      ) : !nsHealth ? (
        <EmptyState>
          Namespace overview is not available for this namespace.
        </EmptyState>
      ) : (
        <>
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
                  : qdrantLoading
                    ? 'Qdrant dense collection status is still loading.'
                    : 'Qdrant dense collection status is unavailable.'
              }
              action={{ label: 'Catalog', to: `${nsBase}/catalog/items` }}
            />
          </Panel>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-5">
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
              label="Vector Readiness"
              value={qdrantData ? `${readyCollections}/4` : '—'}
              sub={qdrantLoading ? 'loading collections' : 'collections available'}
            />
            <MetricTile
              label="Last Run"
              value={lastRun ? formatRelativeTime(lastRun.started_at) : '—'}
              sub={runStatus?.summary ?? 'no runs yet'}
              subClassName={runStatusClass(runStatus)}
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
                  <div className="py-2 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
                    Recommendation
                  </div>
                  <KeyValueRow label="Max results" value={String(config.max_results)} />
                  <KeyValueRow label="Sparse weight (α)" value={String(config.alpha)} />
                  <KeyValueRow label="Time decay (λ)" value={String(config.lambda)} />
                  <KeyValueRow label="Freshness (γ)" value={String(config.gamma)} />
                  <KeyValueRow label="Seen window" value={`${config.seen_items_days}d`} />
                  <div className="py-2 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
                    Dense
                  </div>
                  <KeyValueRow label="Strategy" value={config.dense_strategy} />
                  <KeyValueRow label="Embedding dim" value={String(config.embedding_dim)} />
                  <KeyValueRow label="Distance" value={config.dense_distance} />
                  <div className="py-2 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
                    Trending
                  </div>
                  <KeyValueRow label="Trending window" value={`${config.trending_window}h`} />
                  <KeyValueRow label="Trending TTL" value={`${config.trending_ttl}s`} />
                  <KeyValueRow label="Trending decay (λ)" value={String(config.lambda_trending)} />
                  <div className="py-2 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
                    Namespace
                  </div>
                  <KeyValueRow label="API key" value={config.has_api_key ? 'present' : 'missing'} />
                  <KeyValueRow label="Updated" value={formatDateTime(config.updated_at)} />
                </KeyValueList>
              ) : (
                <LoadingState />
              )}
            </Panel>
          </div>

          <Panel title="Recent Batch Runs" actions={<RunNowButton ns={namespace} />} bodyClassName={recentRuns.length > 0 ? 'overflow-x-auto' : ''}>
            {runsLoading && !runsData ? (
              <LoadingState label="Loading batch runs..." />
            ) : recentRuns.length > 0 ? (
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
            ) : (
              <EmptyState className="p-6">
                No batch runs yet.
              </EmptyState>
            )}
          </Panel>
        </>
      )}
    </PageShell>
  )
}
