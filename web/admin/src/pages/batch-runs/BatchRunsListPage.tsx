import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Card,
  CardContent,
  Container,
  EmptyState,
  Inline,
  Pagination,
  Select,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from '@jarviisha/davinci-react-ui'
import { useBatchRuns, useBatchRunStats, type BatchRunsFilter } from '@/services/batchRuns'
import PageHeader from '@/components/shell/PageHeader'
import PhaseStrip from '@/components/monitoring/PhaseStrip'
import TimeSeriesChart from '@/components/charts/TimeSeriesChart'
import MetaLine from '@/components/MetaLine'
import NamespaceTag from '@/components/NamespaceTag'

const PAGE_SIZE = 25

/**
 * BatchRunsListPage powers both /batch-runs (global, all namespaces) and
 * /ns/:ns/batch-runs (single-namespace). The :ns route param drives the
 * filter — no separate component needed.
 */
export default function BatchRunsListPage() {
  const { ns } = useParams<{ ns?: string }>()
  const [statusFilter, setStatusFilter] = useState<'' | 'running' | 'ok' | 'failed'>('')
  const [kindFilter, setKindFilter] = useState<'' | 'cf' | 'reembed'>('')
  const [page, setPage] = useState(0)

  const filter: BatchRunsFilter = useMemo(
    () => ({
      namespace: ns,
      status: statusFilter,
      kind: kindFilter,
      limit: PAGE_SIZE,
      offset: page * PAGE_SIZE,
    }),
    [ns, statusFilter, kindFilter, page],
  )

  const list = useBatchRuns(filter, { refetchInterval: 15_000 })
  const stats = useBatchRunStats('24h', '1h')

  const seriesData = (stats.data?.series ?? []).map((b) => ({
    ts: b.ts,
    ok: b.ok,
    failed: b.failed,
    cancelled: b.cancelled,
  }))

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Stack gap="050">
          <h1 className="text-foreground text-xl font-semibold">Batch runs</h1>
          <p className="text-foreground-subtle text-sm">
            cron + manual + admin re-embed runs. Newest first; refreshes every 15 seconds.
          </p>
        </Stack>
      </PageHeader>

      <Stack>
        <StatsRow stats={list.data?.stats} />

        {stats.isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : seriesData.length === 0 ? (
          <p className="text-foreground-subtle text-sm">No completed runs in the last 24h.</p>
        ) : (
          <TimeSeriesChart
            data={seriesData}
            series={[
              { key: 'ok', label: 'OK', color: 'var(--davinci-semantic-color-success)' },
              { key: 'failed', label: 'Failed', color: 'var(--davinci-semantic-color-danger)' },
              { key: 'cancelled', label: 'Cancelled', color: 'var(--davinci-semantic-color-warning)' },
            ]}
            stacked
            height={180}
          />
        )}

        <Stack>
          <Inline align="center" justify="between" wrap>
                <Inline align="center">
                  <Select
                    size="sm"
                    value={statusFilter}
                    onChange={(e) => {
                      setStatusFilter(e.target.value as typeof statusFilter)
                      setPage(0)
                    }}
                  >
                    <option value="">all statuses</option>
                    <option value="running">running</option>
                    <option value="ok">ok</option>
                    <option value="failed">failed</option>
                  </Select>
                  <Select
                    size="sm"
                    value={kindFilter}
                    onChange={(e) => {
                      setKindFilter(e.target.value as typeof kindFilter)
                      setPage(0)
                    }}
                  >
                    <option value="">all kinds</option>
                    <option value="cf">cf</option>
                    <option value="reembed">reembed</option>
                  </Select>
                </Inline>
                {list.data && (
                  <MetaLine items={[`${list.data.total} matching`, `page ${page + 1}`]} />
                )}
              </Inline>

              {list.isLoading && <Skeleton className="h-48 w-full" />}

              {list.isError && (
                <Alert variant="danger" title="Failed to load batch runs" description={list.error?.message ?? ''} />
              )}

              {list.isSuccess && list.data.items.length === 0 && (
                <EmptyState
                  title="No runs match"
                  description="Try clearing the filters or wait for the next cron tick."
                />
              )}

              {list.isSuccess && list.data.items.length > 0 && (
                <TableContainer>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead align="right">#</TableHead>
                        {!ns && <TableHead>Namespace</TableHead>}
                        <TableHead>Kind</TableHead>
                        <TableHead>Trigger</TableHead>
                        <TableHead>Started</TableHead>
                        <TableHead align="right">Duration</TableHead>
                        <TableHead>Phases</TableHead>
                        <TableHead>Status</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {list.data.items.map((r) => (
                        <TableRow key={r.id}>
                          <TableCell align="right" className="tabular-nums">
                            <Link
                              to={ns ? `/ns/${encodeURIComponent(ns)}/batch-runs/${r.id}` : `/batch-runs/${r.id}`}
                              className="text-foreground font-medium"
                            >
                              {r.id}
                            </Link>
                          </TableCell>
                          {!ns && (
                            <TableCell>
                              <Link
                                to={`/ns/${encodeURIComponent(r.namespace)}`}
                              >
                                <NamespaceTag name={r.namespace} />
                              </Link>
                            </TableCell>
                          )}
                          <TableCell>
                            <Badge variant={r.kind === 'reembed' ? 'discovery' : 'neutral'}>{r.kind}</Badge>
                          </TableCell>
                          <TableCell className="text-foreground-subtle text-sm">{r.trigger_source}</TableCell>
                          <TableCell className="text-foreground-subtle text-sm">
                            {new Date(r.started_at).toLocaleString()}
                          </TableCell>
                          <TableCell align="right" className="tabular-nums">
                            {r.duration_ms != null ? `${(r.duration_ms / 1000).toFixed(1)}s` : '—'}
                          </TableCell>
                          <TableCell>
                            <PhaseStrip phaseStatus={r.phase_status} />
                          </TableCell>
                          <TableCell>
                            <RunStatusBadge run={r} />
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}

          {list.data && list.data.total > PAGE_SIZE && (
            <Inline justify="end">
              <Pagination
                page={page + 1}
                pageCount={Math.max(1, Math.ceil(list.data.total / PAGE_SIZE))}
                onPageChange={(p) => setPage(p - 1)}
              />
            </Inline>
          )}
        </Stack>
      </Stack>
    </Container>
  )
}

function StatsRow({ stats }: { stats?: { total: number; running: number; ok: number; failed: number } }) {
  const tiles = [
    { label: 'Total', value: stats?.total ?? 0, tone: 'neutral' as const },
    { label: 'Running', value: stats?.running ?? 0, tone: stats?.running ? ('warning' as const) : ('neutral' as const) },
    { label: 'OK', value: stats?.ok ?? 0, tone: 'success' as const },
    { label: 'Failed', value: stats?.failed ?? 0, tone: stats?.failed ? ('danger' as const) : ('neutral' as const) },
  ]
  return (
    <Inline align="start" wrap>
      {tiles.map((t) => (
        <Card key={t.label} className="flex-1 min-w-35">
          <CardContent>
            <Stack>
              <span className="text-foreground-subtle text-xs uppercase tracking-wide">{t.label}</span>
              <Inline align="center">
                <span className="text-foreground text-xl font-semibold tabular-nums">{t.value}</span>
                <Badge variant={t.tone}>{t.tone}</Badge>
              </Inline>
            </Stack>
          </CardContent>
        </Card>
      ))}
    </Inline>
  )
}

function RunStatusBadge({
  run,
}: {
  run: { completed_at: string | null; success: boolean; cancel_requested: boolean; error_message: string | null }
}) {
  if (run.completed_at == null) {
    return <Badge variant={run.cancel_requested ? 'warning' : 'primary'}>{run.cancel_requested ? 'cancelling' : 'running'}</Badge>
  }
  if (run.error_message === 'operator_cancelled') {
    return <Badge variant="neutral">cancelled</Badge>
  }
  if (run.success) {
    return <Badge variant="success">ok</Badge>
  }
  return <Badge variant="danger">failed</Badge>
}
