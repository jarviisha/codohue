import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Banner,
  EmptyState,
  Pagination,
  Selector,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  Token,
} from '@astryxdesign/core'
import {
  kindTokenColor,
  runningTokenProps,
  useBatchRuns,
  useBatchRunStats,
  type BatchRunsFilter,
} from '@/services/batchRuns'
import PageHeader from '@/components/shell/PageHeader'
import PhaseStrip from '@/components/monitoring/PhaseStrip'
import TimeSeriesChart from '@/components/charts/TimeSeriesChart'
import MetaLine from '@/components/MetaLine'
import NamespaceTag from '@/components/NamespaceTag'
import StatTile from '@/components/StatTile'

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
    <>
      <PageHeader>
        <Stack gap={1}>
          <h1 className="text-primary text-xl font-semibold">Batch runs</h1>
          <p className="text-secondary text-sm">
            cron + manual + admin re-embed runs. Newest first; refreshes every 15 seconds.
          </p>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        <StatsRow stats={list.data?.stats} />

        {stats.isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : seriesData.length === 0 ? (
          <p className="text-secondary text-sm">No completed runs in the last 24h.</p>
        ) : (
          <TimeSeriesChart
            data={seriesData}
            series={[
              { key: 'ok', label: 'OK', color: 'var(--color-success)' },
              { key: 'failed', label: 'Failed', color: 'var(--color-error)' },
              { key: 'cancelled', label: 'Cancelled', color: 'var(--color-warning)' },
            ]}
            stacked
            height={180}
          />
        )}

        <Stack gap={6}>
          <Stack gap={4} direction="horizontal" align="center" justify="between" wrap="wrap">
                <Stack gap={4} direction="horizontal" align="center">
                  <Selector
                    size="sm"
                    label="Status"
                    isLabelHidden
                    value={statusFilter}
                    onChange={(next) => {
                      setStatusFilter(next as typeof statusFilter)
                      setPage(0)
                    }}
                    options={[
                      { value: '', label: 'all statuses' },
                      { value: 'running', label: 'running' },
                      { value: 'ok', label: 'ok' },
                      { value: 'failed', label: 'failed' },
                    ]}
                  />
                  <Selector
                    size="sm"
                    label="Kind"
                    isLabelHidden
                    value={kindFilter}
                    onChange={(next) => {
                      setKindFilter(next as typeof kindFilter)
                      setPage(0)
                    }}
                    options={[
                      { value: '', label: 'all kinds' },
                      { value: 'cf', label: 'cf' },
                      { value: 'reembed', label: 'reembed' },
                    ]}
                  />
                </Stack>
                {list.data && (
                  <MetaLine items={[`${list.data.total} matching`, `page ${page + 1}`]} />
                )}
              </Stack>

              {list.isLoading && <Skeleton height={192} />}

              {list.isError && (
                <Banner status="error" title="Failed to load batch runs" description={list.error?.message ?? ''} />
              )}

              {list.isSuccess && list.data.items.length === 0 && (
                <EmptyState
                  title="No runs match"
                  description="Try clearing the filters or wait for the next cron tick."
                />
              )}

              {list.isSuccess && list.data.items.length > 0 && (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHeaderCell className="text-right" >#</TableHeaderCell>
                        {!ns && <TableHeaderCell>Namespace</TableHeaderCell>}
                        <TableHeaderCell>Kind</TableHeaderCell>
                        <TableHeaderCell>Trigger</TableHeaderCell>
                        <TableHeaderCell>Started</TableHeaderCell>
                        <TableHeaderCell className="text-right" >Duration</TableHeaderCell>
                        <TableHeaderCell>Phases</TableHeaderCell>
                        <TableHeaderCell>Status</TableHeaderCell>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {list.data.items.map((r) => (
                        <TableRow key={r.id}>
                          <TableCell  className="text-right tabular-nums">
                            <Link
                              to={ns ? `/ns/${encodeURIComponent(ns)}/batch-runs/${r.id}` : `/batch-runs/${r.id}`}
                              className="text-primary font-medium"
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
                            <Token color={kindTokenColor(r.kind)} label={r.kind} />
                          </TableCell>
                          <TableCell className="text-secondary text-sm">{r.trigger_source}</TableCell>
                          <TableCell className="text-secondary text-sm">
                            {new Date(r.started_at).toLocaleString()}
                          </TableCell>
                          <TableCell  className="text-right tabular-nums">
                            {r.duration_ms != null ? `${(r.duration_ms / 1000).toFixed(1)}s` : '—'}
                          </TableCell>
                          <TableCell>
                            <PhaseStrip phaseStatus={r.phase_status} />
                          </TableCell>
                          <TableCell>
                            <RunStateToken run={r} />
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
              )}

          {list.data && list.data.total > PAGE_SIZE && (
            <Stack align="center" gap={4} direction="horizontal" justify="end">
              <Pagination
                page={page + 1}
                totalPages={Math.max(1, Math.ceil(list.data.total / PAGE_SIZE))}
                onChange={(p) => setPage(p - 1)}
              />
            </Stack>
          )}
        </Stack>
      </Stack>
    </>
  )
}

function StatsRow({ stats }: { stats?: { total: number; running: number; ok: number; failed: number } }) {
  return (
    <Stack gap={4} direction="horizontal" align="start" wrap="wrap">
      <StatTile label="Total" value={stats?.total ?? 0} />
      <StatTile
        label="Running"
        value={stats?.running ?? 0}
        tone="warning"
        hint={stats?.running ? 'active' : undefined}
      />
      <StatTile label="OK" value={stats?.ok ?? 0} />
      <StatTile
        label="Failed"
        value={stats?.failed ?? 0}
        tone="error"
        hint={stats?.failed ? 'attention' : undefined}
      />
    </Stack>
  )
}

function RunStateToken({
  run,
}: {
  run: { completed_at: string | null; success: boolean; cancel_requested: boolean; error_message: string | null }
}) {
  if (run.completed_at == null) {
    return <Token {...runningTokenProps(run.cancel_requested)} />
  }
  if (run.error_message === 'operator_cancelled') {
    return <Token color="gray" label="cancelled" />
  }
  if (run.success) {
    return <Token color="green" label="ok" />
  }
  return <Token color="red" label="failed" />
}
