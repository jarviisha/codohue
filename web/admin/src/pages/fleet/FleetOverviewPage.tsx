import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Badge,
  Banner,
  Layout,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from '@astryxdesign/core'
import { useOverview, type NamespaceOverview, type NamespaceStatus } from '@/services/overview'
import { useBatchRunStats } from '@/services/batchRuns'
import { useMetricsSummary, sumRates } from '@/services/metrics'
import { streamBadgeProps, useServerStream } from '@/services/stream'
import PageHeader from '@/components/shell/PageHeader'
import PhaseStrip from '@/components/monitoring/PhaseStrip'
import TimeSeriesChart from '@/components/charts/TimeSeriesChart'
import MetaLine from '@/components/MetaLine'
import NamespaceTag from '@/components/NamespaceTag'
import StatTile from '@/components/StatTile'

const STATUS_BADGE: Record<NamespaceStatus, { variant: 'success' | 'warning' | 'error' | 'neutral'; label: string }> = {
  active: { variant: 'success', label: 'active' },
  idle: { variant: 'neutral', label: 'idle' },
  degraded: { variant: 'error', label: 'degraded' },
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
      <Layout height="auto" contentWidth={1152}>
        <Skeleton height={192} />
      </Layout>
    )
  }

  if (overview.isError) {
    return (
      <Layout height="auto" contentWidth={1152}>
        <Banner
          status="error"
          title="Could not load fleet overview"
          description={overview.error?.message ?? 'unknown error'}
        />
      </Layout>
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
    <>
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full">
          <Stack gap={1}>
            <h1 className="text-primary text-xl font-semibold">Fleet</h1>
            <MetaLine
              items={[
                `${data.namespaces.length} namespace${data.namespaces.length === 1 ? '' : 's'}`,
                <Badge {...streamBadgeProps(streamConnected)} />,
              ]}
            />
          </Stack>
          {recentRunEvents.length > 0 && (
            <Stack gap={4} direction="horizontal" align="center">
              <span className="text-secondary text-xs">recent:</span>
              {recentRunEvents.map((e, i) => (
                <Badge key={`${e}-${i}`} variant="neutral" label={e} />
              ))}
            </Stack>
          )}
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        {data.alerts.length > 0 && (
          <Stack gap={6}>
            {data.alerts.map((a, i) => (
              <Banner
                key={`${a.kind}-${a.namespace ?? 'global'}-${i}`}
                status={a.level === 'error' ? 'error' : 'warning'}
                title={a.kind.replace(/_/g, ' ')}
                description={`${a.namespace ? `${a.namespace}: ` : ''}${a.message}`}
              />
            ))}
          </Stack>
        )}

        <SummaryRow data={data} />

        <Stack gap={6}>
          <Stack gap={6}>
            <h2 className="text-primary text-sm font-semibold">Batch runs (last 24h)</h2>
            <p className="text-secondary text-xs">
              OK, failed, and cancelled cron + manual runs aggregated per hour.
            </p>
          </Stack>
          {stats.isLoading ? (
            <Skeleton height={192} />
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
              height={220}
            />
          )}
        </Stack>

        <Stack gap={6}>
          <h2 className="text-primary text-sm font-semibold">Namespaces</h2>
          <NamespacesTable namespaces={data.namespaces} />
        </Stack>
      </Stack>
    </>
  )
}

function SummaryRow({ data }: { data: ReturnType<typeof useOverview>['data'] }) {
  // Fleet-wide ingest rate comes from the rolling-window metrics summary, not
  // the events table — keeps the tile cheap and live.
  const metrics = useMetricsSummary()
  if (!data) return null
  const ingestPerSec = sumRates(metrics.data?.ingest.events_per_sec_1m)
  return (
    <Stack gap={4} direction="horizontal" align="start" wrap="wrap">
      <StatTile
        label="Health"
        value={data.health.status}
        tone={data.health.status === 'ok' ? 'success' : 'error'}
      />
      <StatTile
        label="Ingest events/s"
        value={ingestPerSec.toFixed(1)}
        tone={ingestPerSec > 0 ? 'success' : 'neutral'}
        hint={ingestPerSec > 0 ? 'live' : 'idle'}
      />
      <StatTile
        label="Cron heartbeat"
        value={data.cron_heartbeat.ok ? `${data.cron_heartbeat.lag_seconds}s ago` : 'no signal'}
        tone={data.cron_heartbeat.ok ? 'success' : 'warning'}
        hint={data.cron_heartbeat.ok ? 'healthy' : 'stalled'}
      />
      <StatTile
        label="Embedder"
        value={data.embedder_heartbeat.ok ? 'ok' : 'silent'}
        tone={data.embedder_heartbeat.ok ? 'success' : 'warning'}
      />
      <StatTile
        label="Alerts"
        value={data.alerts.length}
        tone={data.alerts.length === 0 ? 'success' : 'warning'}
        hint={data.alerts.length === 0 ? 'clear' : 'attention'}
      />
    </Stack>
  )
}

function NamespacesTable({ namespaces }: { namespaces: NamespaceOverview[] }) {
  if (namespaces.length === 0) {
    return <p className="text-secondary text-sm">No namespaces yet.</p>
  }
  return (
      <Table>
        <TableHeader>
          <TableRow>
            <TableHeaderCell>Namespace</TableHeaderCell>
            <TableHeaderCell>Status</TableHeaderCell>
            <TableHeaderCell>Last run</TableHeaderCell>
            <TableHeaderCell>Phases</TableHeaderCell>
            <TableHeaderCell className="text-right" >Events 24h</TableHeaderCell>
            <TableHeaderCell className="text-right" >Catalog</TableHeaderCell>
          </TableRow>
        </TableHeader>
        <TableBody>
          {namespaces.map((ns) => {
            const status = STATUS_BADGE[ns.status] ?? { variant: 'neutral' as const, label: ns.status }
            return (
              <TableRow key={ns.namespace}>
                <TableCell>
                  <Link to={`/ns/${encodeURIComponent(ns.namespace)}`} className="font-medium">
                    <NamespaceTag name={ns.namespace} />
                  </Link>
                </TableCell>
                <TableCell>
                  <Badge variant={status.variant} label={status.label} />
                </TableCell>
                <TableCell>
                  {ns.last_run ? (
                    <Link
                      to={`/batch-runs/${ns.last_run.id}`}
                      className="text-secondary text-sm"
                    >
                      {new Date(ns.last_run.started_at).toLocaleString()}
                    </Link>
                  ) : (
                    <span className="text-secondary text-sm">—</span>
                  )}
                </TableCell>
                <TableCell>
                  {ns.last_run ? (
                    <PhaseStrip phaseStatus={ns.last_run.phase_status} />
                  ) : (
                    <span className="text-secondary text-sm">—</span>
                  )}
                </TableCell>
                <TableCell  className="text-right tabular-nums">
                  {ns.events_24h.toLocaleString()}
                </TableCell>
                <TableCell className="text-right" >
                  {ns.catalog.enabled ? (
                    <Stack gap={4} direction="horizontal" align="center" justify="end">
                      <Badge variant={ns.catalog.dead_letter > 0 ? 'error' : 'neutral'} label={<>{ns.catalog.pending} pending</>} />
                      {ns.catalog.dead_letter > 0 && (
                        <Badge variant="error" label={<>{ns.catalog.dead_letter} DL</>} />
                      )}
                    </Stack>
                  ) : (
                    <span className="text-secondary text-sm">off</span>
                  )}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
  )
}
