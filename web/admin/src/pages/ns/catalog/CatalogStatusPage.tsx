import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Badge,
  Banner,
  Button,
  Card,
  EmptyState,
  ProgressBar,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from '@astryxdesign/core'
import {
  useBulkRedriveDeadletter,
  useCatalogBacklogHistory,
  useCatalogConfig,
  useCatalogFailuresSummary,
  useTriggerReEmbed,
  type CatalogBacklog,
} from '@/services/catalog'
import { streamBadgeProps, useServerStream } from '@/services/stream'
import PageHeader from '@/components/shell/PageHeader'
import TimeSeriesChart from '@/components/charts/TimeSeriesChart'
import MetaLine from '@/components/MetaLine'
import StatTile from '@/components/StatTile'

const HISTORY_WINDOWS = ['1h', '24h', '7d'] as const
type HistoryWindow = (typeof HISTORY_WINDOWS)[number]

type ReembedProgress = {
  batch_run_id: number
  processed: number
  total: number
  at: string
}

export default function CatalogStatusPage() {
  const { ns } = useParams<{ ns: string }>()
  const [window, setWindow] = useState<HistoryWindow>('1h')
  const [streamEvents, setStreamEvents] = useState(0)

  const config = useCatalogConfig(ns ?? null)
  const history = useCatalogBacklogHistory(ns ?? null, window)
  const failures = useCatalogFailuresSummary(ns ?? null, '24h')
  const reembed = useTriggerReEmbed(ns ?? null)
  const bulkRedrive = useBulkRedriveDeadletter(ns ?? null)

  // Live backlog snapshot from SSE — when present we render it on the tiles
  // so the page tracks the embedder's sample cadence (30s) rather than the
  // /catalog poll interval (15s). embedded + consumer_lag aren't carried on
  // the SSE event; they come from the polled config response.
  const [liveBacklog, setLiveBacklog] = useState<CatalogBacklog | null>(null)
  const [dlAlert, setDlAlert] = useState<{
    namespace: string
    new_count: number
    delta: number
  } | null>(null)
  // Per-batch reembed_progress samples. Keyed by batch_run_id so multiple
  // concurrent runs each get their own tick — the watcher emits one event
  // per open run per 5s.
  const [progressByRun, setProgressByRun] = useState<Record<number, ReembedProgress>>({})

  const { connected: streamConnected } = useServerStream(
    ns ? `/api/admin/v1/namespaces/${ns}/catalog/stream` : null,
    useMemo(
      () => ({
        item_state_changed: () => setStreamEvents((n) => n + 1),
        backlog_snapshot: (data: unknown) => {
          const d = data as Record<string, number | undefined>
          setLiveBacklog((prev) => ({
            pending: d.pending ?? 0,
            in_flight: d.in_flight ?? 0,
            failed: d.failed ?? 0,
            dead_letter: d.dead_letter ?? 0,
            stream_len: d.stream_len ?? 0,
            embedded: prev?.embedded ?? 0,
            consumer_lag: prev?.consumer_lag ?? 0,
          }))
          setStreamEvents((n) => n + 1)
        },
        dead_letter_grew: (data: unknown) => {
          const d = data as { namespace?: string; new_count?: number; delta?: number }
          setDlAlert({
            namespace: d.namespace ?? ns ?? '',
            new_count: d.new_count ?? 0,
            delta: d.delta ?? 0,
          })
          setStreamEvents((n) => n + 1)
        },
        // ReembedWatcher emits one of these per open run per 5s tick. We
        // keep a small map keyed by batch_run_id so the Last re-embed card
        // and any later widget can render live processed/total.
        reembed_progress: (data: unknown) => {
          const d = data as {
            batch_run_id?: number
            processed?: number
            total?: number
            at?: string
          }
          if (d.batch_run_id == null) return
          setProgressByRun((m) => ({
            ...m,
            [d.batch_run_id!]: {
              batch_run_id: d.batch_run_id!,
              processed: d.processed ?? 0,
              total: d.total ?? 0,
              at: d.at ?? new Date().toISOString(),
            },
          }))
          setStreamEvents((n) => n + 1)
        },
      }),
      [ns],
    ),
  )

  if (!ns) return null

  if (config.isLoading) {
    return <Skeleton height={192} />
  }

  if (config.isError) {
    return (
      <Banner
        status="error"
        title="Could not load catalog config"
        description={config.error?.message ?? 'unknown error'}
      />
    )
  }

  const data = config.data

  if (!data || !data.catalog.enabled) {
    return (
      <>
        <PageHeader>
          <Stack
            gap={4}
            direction="horizontal"
            align="center"
            justify="between"
            className="w-full"
            wrap="wrap"
          >
            <Stack gap={1}>
              <h1 className="text-primary text-xl font-semibold">Catalog</h1>
              <p className="text-secondary text-sm">Auto-embedding is currently off.</p>
            </Stack>
            <Button
              size="sm"
              href={`/ns/${encodeURIComponent(ns)}/config?tab=embeddings`}
              label="Enable catalog"
            />
          </Stack>
        </PageHeader>
        <EmptyState
          title="Catalog auto-embedding is off"
          description="Enable it in the namespace config to start ingesting raw content for embedding."
        />
      </>
    )
  }

  // Merge live SSE deltas onto the polled snapshot. SSE only carries the
  // four backlog states + stream_len, so embedded + consumer_lag come from
  // the polled response.
  const backlog: CatalogBacklog = liveBacklog
    ? {
        ...liveBacklog,
        embedded: data.backlog.embedded,
        consumer_lag: data.backlog.consumer_lag,
      }
    : data.backlog
  const reembedStatus = data.last_re_embed
  const reembedRunning = reembedStatus?.status === 'running'

  return (
    <>
      <PageHeader>
        <Stack
          gap={4}
          direction="horizontal"
          align="center"
          justify="between"
          className="w-full"
          wrap="wrap"
        >
          <Stack gap={6}>
            <Stack gap={4} direction="horizontal" align="center">
              <h1 className="text-primary text-xl font-semibold">Catalog</h1>
              <Badge {...streamBadgeProps(streamConnected)} />
              {streamEvents > 0 && (
                <span className="text-secondary text-xs tabular-nums">
                  {streamEvents} live event{streamEvents === 1 ? '' : 's'}
                </span>
              )}
            </Stack>
            <p className="text-secondary text-sm">
              strategy={data.catalog.strategy_id}@{data.catalog.strategy_version}
            </p>
          </Stack>
          <Stack gap={4} direction="horizontal" align="center">
            <Button
              size="sm"
              variant="secondary"
              href={`/ns/${encodeURIComponent(ns)}/config?tab=embeddings`}
              label="Catalog settings"
            />

            {backlog.dead_letter > 0 && (
              <Button
                size="sm"
                variant="destructive"
                onClick={() => bulkRedrive.mutate()}
                isDisabled={bulkRedrive.isPending}
                label={
                  bulkRedrive.isPending
                    ? 'Redriving…'
                    : `Redrive ${backlog.dead_letter} dead-letter`
                }
              />
            )}
            <Button
              size="sm"
              onClick={() => reembed.mutate()}
              isDisabled={reembed.isPending || reembedRunning}
              label={
                reembedRunning
                  ? 'Re-embed running…'
                  : reembed.isPending
                    ? 'Starting…'
                    : 'Trigger re-embed'
              }
            />
          </Stack>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        {dlAlert && dlAlert.delta > 0 && (
          <Banner
            status="error"
            title={`Dead-letter grew by ${dlAlert.delta}`}
            description={`Namespace ${dlAlert.namespace} now has ${dlAlert.new_count} dead-letter items. Investigate or bulk-redrive once the root cause is fixed.`}
            endContent={
              <Button size="sm" variant="ghost" onClick={() => setDlAlert(null)} label="Dismiss" />
            }
          />
        )}
        {reembed.error && (
          <Banner status="error" title="Re-embed failed" description={reembed.error.message} />
        )}
        {bulkRedrive.error && (
          <Banner
            status="error"
            title="Bulk redrive failed"
            description={bulkRedrive.error.message}
          />
        )}

        <BacklogTiles backlog={backlog} />

        {reembedStatus && (
          <Card>
            <Stack gap={6}>
              <Stack gap={4} direction="horizontal" align="center" justify="between">
                <Stack gap={4} direction="horizontal" align="center">
                  <span className="text-secondary text-xs uppercase tracking-wide">
                    Last re-embed
                  </span>
                  <ReembedStatusBadge status={reembedStatus.status} />
                </Stack>
                <Link
                  to={`/ns/${encodeURIComponent(ns)}/batch-runs/${reembedStatus.batch_run_id}`}
                  className="text-primary text-sm font-medium"
                >
                  #{reembedStatus.batch_run_id} →
                </Link>
              </Stack>
              <MetaLine
                size="xs"
                items={[
                  `strategy=${reembedStatus.strategy_id}@${reembedStatus.strategy_version}`,
                  `started ${new Date(reembedStatus.started_at).toLocaleString()}`,
                  reembedStatus.duration_ms != null &&
                    `${(reembedStatus.duration_ms / 1000).toFixed(1)}s`,
                  reembedStatus.processed != null &&
                    `processed ${reembedStatus.processed.toLocaleString()}`,
                ]}
              />
              {reembedStatus.error_message && (
                <span className="text-error text-xs">{reembedStatus.error_message}</span>
              )}
              {reembedRunning && progressByRun[reembedStatus.batch_run_id] && (
                <ReembedProgressBar progress={progressByRun[reembedStatus.batch_run_id]} />
              )}
            </Stack>
          </Card>
        )}

        <Stack gap={6}>
          <Stack gap={4} direction="horizontal" align="center" justify="between">
            <Stack gap={6}>
              <h2 className="text-primary text-sm font-semibold">Backlog timeline</h2>
              <p className="text-secondary text-xs">
                Persisted samples — survives reload, sampled every 30 seconds.
              </p>
            </Stack>
            <Stack align="center" gap={4} direction="horizontal">
              {HISTORY_WINDOWS.map((w) => (
                <Button
                  key={w}
                  size="sm"
                  variant="primary"
                  onClick={() => setWindow(w)}
                  label={w}
                />
              ))}
            </Stack>
          </Stack>
          {history.isLoading ? (
            <Skeleton className="h-40 w-full" />
          ) : history.data?.samples.length === 0 ? (
            <p className="text-secondary text-sm">No samples yet — the sampler writes every 30s.</p>
          ) : (
            <TimeSeriesChart
              data={(history.data?.samples ?? []).map((s) => ({
                ts: s.sampled_at,
                pending: s.pending,
                in_flight: s.in_flight,
                failed: s.failed,
                dead_letter: s.dead_letter,
              }))}
              series={[
                {
                  key: 'pending',
                  label: 'Pending',
                  color: 'var(--color-text-secondary)',
                },
                {
                  key: 'in_flight',
                  label: 'In flight',
                  color: 'var(--color-text-blue)',
                },
                { key: 'failed', label: 'Failed', color: 'var(--color-warning)' },
                {
                  key: 'dead_letter',
                  label: 'Dead-letter',
                  color: 'var(--color-error)',
                },
              ]}
              stacked
              height={240}
            />
          )}
        </Stack>

        <Stack gap={6}>
          <Stack gap={6}>
            <h2 className="text-primary text-sm font-semibold">Top failure reasons (24h)</h2>
            <p className="text-secondary text-xs">
              Buckets failed + dead-letter rows by last_error so the dominant cause surfaces first.
            </p>
          </Stack>
          {failures.isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : failures.data?.reasons.length === 0 ? (
            <p className="text-secondary text-sm">No failed items in the last 24h.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHeaderCell>Reason</TableHeaderCell>
                  <TableHeaderCell className="text-right">Count</TableHeaderCell>
                  <TableHeaderCell>Sample object</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {failures.data?.reasons.map((r, i) => (
                  <TableRow key={`${r.reason}-${i}`}>
                    <TableCell className="text-secondary text-sm">{r.reason}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {r.count.toLocaleString()}
                    </TableCell>
                    <TableCell>
                      {r.sample_object_id ? (
                        <code className="text-secondary text-xs">{r.sample_object_id}</code>
                      ) : (
                        <span className="text-secondary text-xs">—</span>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Stack>

        <Stack align="center" gap={4} direction="horizontal" justify="end">
          <Button
            href={`/ns/${encodeURIComponent(ns)}/catalog/items`}
            variant="secondary"
            label="Browse items →"
          />
        </Stack>
      </Stack>
    </>
  )
}

function ReembedProgressBar({ progress }: { progress: ReembedProgress }) {
  const total = progress.total > 0 ? progress.total : 1
  const pct = Math.min(100, Math.round((progress.processed / total) * 100))
  return (
    <Stack gap={6}>
      <Stack gap={4} direction="horizontal" align="center" justify="between">
        <span className="text-secondary text-xs tabular-nums">
          {progress.processed.toLocaleString()} / {progress.total.toLocaleString()} ({pct}%)
        </span>
        <span className="text-secondary text-xs">
          updated {new Date(progress.at).toLocaleTimeString()}
        </span>
      </Stack>
      <ProgressBar
        label="Re-embed progress"
        value={progress.processed}
        max={total}
        isLabelHidden
      />
    </Stack>
  )
}

function BacklogTiles({ backlog }: { backlog: CatalogBacklog }) {
  return (
    <Stack gap={4} direction="horizontal" align="start" wrap="wrap">
      <StatTile label="Pending" value={backlog.pending} />
      <StatTile label="In flight" value={backlog.in_flight} />
      <StatTile
        label="Failed"
        value={backlog.failed}
        tone="warning"
        hint={backlog.failed > 0 ? 'attention' : undefined}
      />
      <StatTile
        label="Dead-letter"
        value={backlog.dead_letter}
        tone="error"
        hint={backlog.dead_letter > 0 ? 'stuck' : undefined}
      />
      <StatTile label="Embedded" value={backlog.embedded} />
      <StatTile label="Stream length (XLEN)" value={backlog.stream_len} />
      <StatTile
        label="Consumer lag (PEL)"
        value={backlog.consumer_lag}
        tone="warning"
        hint={backlog.consumer_lag > 1000 ? 'high' : undefined}
      />
    </Stack>
  )
}

function ReembedStatusBadge({ status }: { status: string }) {
  if (status === 'running') return <Badge variant="info" label="running" />
  if (status === 'success') return <Badge variant="success" label="ok" />
  if (status === 'failed') return <Badge variant="error" label="failed" />
  return <Badge variant="neutral" label={status} />
}
