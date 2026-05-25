import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Alert,
  Badge,
  Card,
  CardContent,
  Container,
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
import { useOverview, type NamespaceOverview, type NamespaceStatus } from '@/services/overview'
import { useBatchRunStats } from '@/services/batchRuns'
import { useServerStream } from '@/services/stream'
import PageHeader from '@/components/shell/PageHeader'
import PhaseStrip from '@/components/monitoring/PhaseStrip'
import TimeSeriesChart from '@/components/charts/TimeSeriesChart'

const STATUS_BADGE: Record<NamespaceStatus, { variant: 'success' | 'warning' | 'danger' | 'neutral'; label: string }> = {
  active: { variant: 'success', label: 'active' },
  idle: { variant: 'neutral', label: 'idle' },
  degraded: { variant: 'danger', label: 'degraded' },
  cold: { variant: 'neutral', label: 'cold' },
}

export default function FleetOverviewPage() {
  const overview = useOverview()
  const stats = useBatchRunStats('24h', '1h')

  // /stream subscription drives a "live" pulse on the overview tile + a running
  // count of in-flight runs without forcing the user to wait for the next
  // 30-second poll. We intentionally don't merge the events into the table —
  // the table refetches on its own; the stream is for ambient awareness.
  const [recentRunEvents, setRecentRunEvents] = useState<string[]>([])

  const { connected: streamConnected } = useServerStream(
    '/api/admin/v1/stream',
    useMemo(
      () => ({
        started: (data: unknown) => {
          const ns = (data as { namespace?: string })?.namespace ?? '?'
          setRecentRunEvents((prev) => [`started ${ns}`, ...prev].slice(0, 5))
        },
        completed: (data: unknown) => {
          const ns = (data as { namespace?: string })?.namespace ?? '?'
          const ok = (data as { success?: boolean })?.success
          setRecentRunEvents((prev) => [`${ok ? 'ok' : 'fail'} ${ns}`, ...prev].slice(0, 5))
        },
        cancelled: (data: unknown) => {
          const ns = (data as { namespace?: string })?.namespace ?? '?'
          setRecentRunEvents((prev) => [`cancelled ${ns}`, ...prev].slice(0, 5))
        },
      }),
      [],
    ),
  )

  if (overview.isLoading) {
    return (
      <Container size="lg" className="py-6">
        <Skeleton className="h-48 w-full" />
      </Container>
    )
  }

  if (overview.isError) {
    return (
      <Container size="lg" className="py-6">
        <Alert
          variant="danger"
          title="Could not load fleet overview"
          description={overview.error?.message ?? 'unknown error'}
        />
      </Container>
    )
  }

  const data = overview.data!
  const seriesData = (stats.data?.series ?? []).map((b) => ({
    ts: b.ts,
    ok: b.ok,
    failed: b.failed,
    cancelled: b.cancelled,
  }))

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Inline gap="200" align="center" justify="between" className="w-full">
          <Stack gap="025">
            <h1 className="text-foreground text-xl font-semibold">Fleet</h1>
            <p className="text-foreground-subtle text-sm">
              {data.namespaces.length} namespace{data.namespaces.length === 1 ? '' : 's'} · stream{' '}
              <Badge variant={streamConnected ? 'success' : 'neutral'}>
                {streamConnected ? 'connected' : 'offline'}
              </Badge>
            </p>
          </Stack>
          {recentRunEvents.length > 0 && (
            <Inline gap="050" align="center">
              <span className="text-foreground-subtle text-xs">recent:</span>
              {recentRunEvents.map((e, i) => (
                <Badge key={`${e}-${i}`} variant="neutral">
                  {e}
                </Badge>
              ))}
            </Inline>
          )}
        </Inline>
      </PageHeader>

      <Stack gap="300">
        {data.alerts.length > 0 && (
          <Stack gap="100">
            {data.alerts.map((a, i) => (
              <Alert
                key={`${a.kind}-${a.namespace ?? 'global'}-${i}`}
                variant={a.level === 'error' ? 'danger' : 'warning'}
                title={a.kind.replace(/_/g, ' ')}
                description={`${a.namespace ? `${a.namespace}: ` : ''}${a.message}`}
              />
            ))}
          </Stack>
        )}

        <SummaryRow data={data} />

        <Stack gap="100">
          <Stack gap="025">
            <h2 className="text-foreground text-sm font-semibold">Batch runs · last 24h</h2>
            <p className="text-foreground-subtle text-xs">
              OK, failed, and cancelled cron + manual runs aggregated per hour.
            </p>
          </Stack>
          {stats.isLoading ? (
            <Skeleton className="h-48 w-full" />
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
              height={220}
            />
          )}
        </Stack>

        <Stack gap="100">
          <h2 className="text-foreground text-sm font-semibold">Namespaces</h2>
          <NamespacesTable namespaces={data.namespaces} />
        </Stack>
      </Stack>
    </Container>
  )
}

function SummaryRow({ data }: { data: ReturnType<typeof useOverview>['data'] }) {
  if (!data) return null
  const tiles = [
    {
      label: 'Health',
      value: data.health.status,
      tone: data.health.status === 'ok' ? 'success' : 'danger',
    },
    {
      label: 'Cron heartbeat',
      value: data.cron_heartbeat.ok
        ? `${data.cron_heartbeat.lag_seconds}s ago`
        : 'no signal',
      tone: data.cron_heartbeat.ok ? 'success' : 'warning',
    },
    {
      label: 'Embedder',
      value: data.embedder_heartbeat.ok ? 'ok' : 'silent',
      tone: data.embedder_heartbeat.ok ? 'success' : 'warning',
    },
    {
      label: 'Alerts',
      value: data.alerts.length,
      tone: data.alerts.length === 0 ? 'success' : 'warning',
    },
  ] as const

  return (
    <Inline gap="200" align="start" wrap>
      {tiles.map((t) => (
        <Card key={t.label} className="flex-1 min-w-35">
          <CardContent>
            <Stack gap="025">
              <span className="text-foreground-subtle text-xs uppercase tracking-wide">{t.label}</span>
              <Inline gap="050" align="center">
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

function NamespacesTable({ namespaces }: { namespaces: NamespaceOverview[] }) {
  if (namespaces.length === 0) {
    return <p className="text-foreground-subtle text-sm">No namespaces yet.</p>
  }
  return (
    <TableContainer>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Namespace</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Last run</TableHead>
            <TableHead>Phases</TableHead>
            <TableHead align="right">Events 24h</TableHead>
            <TableHead align="right">Catalog</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {namespaces.map((ns) => {
            const status = STATUS_BADGE[ns.status] ?? { variant: 'neutral' as const, label: ns.status }
            return (
              <TableRow key={ns.namespace}>
                <TableCell>
                  <Link to={`/ns/${encodeURIComponent(ns.namespace)}`} className="text-foreground font-medium">
                    {ns.namespace}
                  </Link>
                </TableCell>
                <TableCell>
                  <Badge variant={status.variant}>{status.label}</Badge>
                </TableCell>
                <TableCell>
                  {ns.last_run ? (
                    <Link
                      to={`/batch-runs/${ns.last_run.id}`}
                      className="text-foreground-subtle text-sm"
                    >
                      {new Date(ns.last_run.started_at).toLocaleString()}
                    </Link>
                  ) : (
                    <span className="text-foreground-subtle text-sm">—</span>
                  )}
                </TableCell>
                <TableCell>
                  {ns.last_run ? (
                    <PhaseStrip phaseStatus={ns.last_run.phase_status} />
                  ) : (
                    <span className="text-foreground-subtle text-sm">—</span>
                  )}
                </TableCell>
                <TableCell align="right" className="tabular-nums">
                  {ns.events_24h.toLocaleString()}
                </TableCell>
                <TableCell align="right">
                  {ns.catalog.enabled ? (
                    <Inline gap="050" align="center" justify="end">
                      <Badge variant={ns.catalog.dead_letter > 0 ? 'danger' : 'neutral'}>
                        {ns.catalog.pending} pending
                      </Badge>
                      {ns.catalog.dead_letter > 0 && (
                        <Badge variant="danger">{ns.catalog.dead_letter} DL</Badge>
                      )}
                    </Inline>
                  ) : (
                    <span className="text-foreground-subtle text-sm">off</span>
                  )}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </TableContainer>
  )
}
