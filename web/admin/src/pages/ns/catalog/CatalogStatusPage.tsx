import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Button,
  Card,
  CardContent,
  Container,
  EmptyState,
  Inline,
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
import {
  useBulkRedriveDeadletter,
  useCatalogBacklogHistory,
  useCatalogConfig,
  useCatalogFailuresSummary,
  useTriggerReEmbed,
  type CatalogBacklog,
} from '@/services/catalog'
import { useServerStream } from '@/services/stream'
import PageHeader from '@/components/shell/PageHeader'
import TimeSeriesChart from '@/components/charts/TimeSeriesChart'

const HISTORY_WINDOWS = ['1h', '24h', '7d'] as const
type HistoryWindow = (typeof HISTORY_WINDOWS)[number]

export default function CatalogStatusPage() {
  const { ns } = useParams<{ ns: string }>()
  const [window, setWindow] = useState<HistoryWindow>('1h')
  const [streamEvents, setStreamEvents] = useState(0)

  const config = useCatalogConfig(ns ?? null)
  const history = useCatalogBacklogHistory(ns ?? null, window)
  const failures = useCatalogFailuresSummary(ns ?? null, '24h')
  const reembed = useTriggerReEmbed(ns ?? null)
  const bulkRedrive = useBulkRedriveDeadletter(ns ?? null)

  // Subscribe to catalog SSE stream while we're on the page. Each
  // item_state_changed event increments a counter so operators see the
  // stream is alive even before the polling refetch lands.
  const { connected: streamConnected } = useServerStream(
    ns ? `/api/admin/v1/namespaces/${ns}/catalog/stream` : null,
    useMemo(
      () => ({
        item_state_changed: () => setStreamEvents((n) => n + 1),
        dead_letter_grew: () => setStreamEvents((n) => n + 1),
      }),
      [],
    ),
  )

  if (!ns) return null

  if (config.isLoading) {
    return (
      <Container size="full" className="py-6 px-6">
        <Skeleton className="h-48 w-full" />
      </Container>
    )
  }

  if (config.isError) {
    return (
      <Container size="full" className="py-6 px-6">
        <Alert
          variant="danger"
          title="Could not load catalog config"
          description={config.error?.message ?? 'unknown error'}
        />
      </Container>
    )
  }

  const data = config.data
  if (!data || !data.catalog.enabled) {
    return (
      <Container size="full" className="py-6 px-6">
        <PageHeader>
          <Stack gap="025">
            <h1 className="text-foreground text-xl font-semibold">Catalog · {ns}</h1>
          </Stack>
        </PageHeader>
        <EmptyState
          title="Catalog auto-embedding is off"
          description="Enable it in the namespace config to start ingesting raw content for embedding."
        />
      </Container>
    )
  }

  const backlog = data.backlog
  const reembedStatus = data.last_re_embed
  const reembedRunning = reembedStatus?.status === 'running'

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Inline gap="200" align="center" justify="between" className="w-full" wrap>
          <Stack gap="025">
            <Inline gap="100" align="center">
              <h1 className="text-foreground text-xl font-semibold">Catalog · {ns}</h1>
              <Badge variant={streamConnected ? 'success' : 'neutral'}>
                stream {streamConnected ? 'connected' : 'offline'}
              </Badge>
              {streamEvents > 0 && (
                <span className="text-foreground-subtle text-xs tabular-nums">
                  {streamEvents} live event{streamEvents === 1 ? '' : 's'}
                </span>
              )}
            </Inline>
            <p className="text-foreground-subtle text-sm">
              strategy={data.catalog.strategy_id}@{data.catalog.strategy_version}
            </p>
          </Stack>
          <Inline gap="100" align="center">
            {backlog.dead_letter > 0 && (
              <Button
                size="sm"
                variant="outline"
                tone="danger"
                onClick={() => bulkRedrive.mutate()}
                disabled={bulkRedrive.isPending}
              >
                {bulkRedrive.isPending
                  ? 'Redriving…'
                  : `Redrive ${backlog.dead_letter} dead-letter`}
              </Button>
            )}
            <Button
              size="sm"
              onClick={() => reembed.mutate()}
              disabled={reembed.isPending || reembedRunning}
            >
              {reembedRunning
                ? 'Re-embed running…'
                : reembed.isPending
                  ? 'Starting…'
                  : 'Trigger re-embed'}
            </Button>
          </Inline>
        </Inline>
      </PageHeader>

      <Stack gap="300">
        {reembed.error && (
          <Alert variant="danger" title="Re-embed failed" description={reembed.error.message} />
        )}
        {bulkRedrive.error && (
          <Alert
            variant="danger"
            title="Bulk redrive failed"
            description={bulkRedrive.error.message}
          />
        )}

        <BacklogTiles backlog={backlog} />

        {reembedStatus && (
          <Card>
            <CardContent>
              <Stack gap="050">
                <Inline gap="100" align="center" justify="between">
                  <Inline gap="100" align="center">
                    <span className="text-foreground-subtle text-xs uppercase tracking-wide">
                      Last re-embed
                    </span>
                    <ReembedStatusBadge status={reembedStatus.status} />
                  </Inline>
                  <Link
                    to={`/batch-runs/${reembedStatus.batch_run_id}`}
                    className="text-foreground text-sm font-medium"
                  >
                    #{reembedStatus.batch_run_id} →
                  </Link>
                </Inline>
                <span className="text-foreground-subtle text-xs">
                  strategy={reembedStatus.strategy_id}@{reembedStatus.strategy_version} · started{' '}
                  {new Date(reembedStatus.started_at).toLocaleString()}
                  {reembedStatus.duration_ms != null &&
                    ` · ${(reembedStatus.duration_ms / 1000).toFixed(1)}s`}
                  {reembedStatus.processed != null && ` · processed ${reembedStatus.processed.toLocaleString()}`}
                </span>
                {reembedStatus.error_message && (
                  <span className="text-danger text-xs">{reembedStatus.error_message}</span>
                )}
              </Stack>
            </CardContent>
          </Card>
        )}

        <Stack gap="100">
          <Inline gap="100" align="center" justify="between">
            <Stack gap="025">
              <h2 className="text-foreground text-sm font-semibold">Backlog timeline</h2>
              <p className="text-foreground-subtle text-xs">
                Persisted samples — survives reload, sampled every 30 seconds.
              </p>
            </Stack>
            <Inline gap="050">
              {HISTORY_WINDOWS.map((w) => (
                <Button
                  key={w}
                  size="sm"
                  variant={window === w ? 'solid' : 'ghost'}
                  tone="neutral"
                  onClick={() => setWindow(w)}
                >
                  {w}
                </Button>
              ))}
            </Inline>
          </Inline>
          {history.isLoading ? (
            <Skeleton className="h-40 w-full" />
          ) : history.data?.samples.length === 0 ? (
            <p className="text-foreground-subtle text-sm">
              No samples yet — the sampler writes every 30s.
            </p>
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
                  color: 'var(--davinci-semantic-color-foreground-subtle)',
                },
                {
                  key: 'in_flight',
                  label: 'In flight',
                  color: 'var(--davinci-color-blue-500)',
                },
                { key: 'failed', label: 'Failed', color: 'var(--davinci-semantic-color-warning)' },
                {
                  key: 'dead_letter',
                  label: 'Dead-letter',
                  color: 'var(--davinci-semantic-color-danger)',
                },
              ]}
              stacked
              height={240}
            />
          )}
        </Stack>

        <Stack gap="100">
          <Stack gap="025">
            <h2 className="text-foreground text-sm font-semibold">Top failure reasons (24h)</h2>
            <p className="text-foreground-subtle text-xs">
              Buckets failed + dead-letter rows by last_error so the dominant cause surfaces first.
            </p>
          </Stack>
          {failures.isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : failures.data?.reasons.length === 0 ? (
            <p className="text-foreground-subtle text-sm">No failed items in the last 24h.</p>
          ) : (
            <TableContainer>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Reason</TableHead>
                    <TableHead align="right">Count</TableHead>
                    <TableHead>Sample object</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {failures.data?.reasons.map((r, i) => (
                    <TableRow key={`${r.reason}-${i}`}>
                      <TableCell className="text-foreground-subtle text-sm">{r.reason}</TableCell>
                      <TableCell align="right" className="tabular-nums">
                        {r.count.toLocaleString()}
                      </TableCell>
                      <TableCell>
                        {r.sample_object_id ? (
                          <code className="text-foreground-subtle text-xs">{r.sample_object_id}</code>
                        ) : (
                          <span className="text-foreground-subtle text-xs">—</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Stack>

        <Inline gap="100" justify="end">
          <Link to={`/ns/${encodeURIComponent(ns)}/catalog/items`}>
            <Button variant="outline" tone="neutral">
              Browse items →
            </Button>
          </Link>
        </Inline>
      </Stack>
    </Container>
  )
}

function BacklogTiles({ backlog }: { backlog: CatalogBacklog }) {
  const tiles: Array<{ label: string; value: number; tone: 'neutral' | 'warning' | 'danger' }> = [
    { label: 'Pending', value: backlog.pending, tone: 'neutral' },
    { label: 'In flight', value: backlog.in_flight, tone: 'neutral' },
    {
      label: 'Failed',
      value: backlog.failed,
      tone: backlog.failed > 0 ? 'warning' : 'neutral',
    },
    {
      label: 'Dead-letter',
      value: backlog.dead_letter,
      tone: backlog.dead_letter > 0 ? 'danger' : 'neutral',
    },
    { label: 'Embedded', value: backlog.embedded, tone: 'neutral' },
    { label: 'Stream length', value: backlog.stream_len, tone: 'neutral' },
  ]
  return (
    <Inline gap="200" align="start" wrap>
      {tiles.map((t) => (
        <Card key={t.label} className="flex-1 min-w-35">
          <CardContent>
            <Stack gap="025">
              <span className="text-foreground-subtle text-xs uppercase tracking-wide">
                {t.label}
              </span>
              <Inline gap="050" align="center">
                <span className="text-foreground text-xl font-semibold tabular-nums">
                  {t.value.toLocaleString()}
                </span>
                {t.tone !== 'neutral' && <Badge variant={t.tone}>!</Badge>}
              </Inline>
            </Stack>
          </CardContent>
        </Card>
      ))}
    </Inline>
  )
}

function ReembedStatusBadge({ status }: { status: string }) {
  if (status === 'running') return <Badge variant="primary">running</Badge>
  if (status === 'success') return <Badge variant="success">ok</Badge>
  if (status === 'failed') return <Badge variant="danger">failed</Badge>
  return <Badge variant="neutral">{status}</Badge>
}
