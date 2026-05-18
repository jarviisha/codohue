import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  Button,
  EmptyState,
  KeyValueList,
  LoadingState,
  MetricTile,
  Notice,
  PageHeader,
  PageShell,
  Panel,
  StatusToken,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
  useRegisterCommand,
} from '@/components/ui'
import type { StatusState } from '@/components/ui'
import { probeState, useHealth } from '@/services/health'
import { useTriggerBatchRun } from '@/services/batchRuns'
import { useCatalogConfig } from '@/services/catalog'
import {
  lastRunToken,
  namespaceStatusToken,
  useNamespace,
  useNamespacesOverview,
} from '@/services/namespaces'
import { useTrending } from '@/services/trending'
import { paths } from '@/routes/path'
import {
  formatDurationMs,
  formatNumber,
  formatTimestamp,
} from '@/utils/format'

function phaseToken(ok: boolean | null | undefined): StatusState {
  if (ok === null || ok === undefined) return 'idle'
  return ok ? 'ok' : 'fail'
}

export default function NamespaceOverviewPage() {
  const { name = '' } = useParams<{ name: string }>()
  const navigate = useNavigate()

  const health = useHealth()
  const overview = useNamespacesOverview()
  const config = useNamespace(name)
  const catalog = useCatalogConfig(name)
  const trending = useTrending({ namespace: name, limit: 5 })
  const triggerBatch = useTriggerBatchRun()

  useRegisterCommand(
    `ns.${name}.batch.run`,
    `Run batch on ${name}`,
    () => {
      if (!triggerBatch.isPending) triggerBatch.mutate(name)
    },
    name,
  )
  useRegisterCommand(
    `ns.${name}.config`,
    `Open ${name} config`,
    () => navigate(paths.nsConfig(name)),
    name,
  )
  useRegisterCommand(
    `ns.${name}.catalog`,
    `Open ${name} catalog`,
    () => navigate(paths.nsCatalog(name)),
    name,
  )

  const thisNs = overview.data?.items.find((it) => it.config.namespace === name)
  const lastRun = thisNs?.last_run ?? null
  const healthData = health.data

  return (
    <PageShell>
      <PageHeader
        title="Overview"
        meta={
          <span className="inline-flex items-center gap-2">
            <span>
              namespace <span className="text-primary">{name}</span>
            </span>
            {thisNs ? (
              <>
                <span className="text-muted">·</span>
                <StatusToken
                  state={namespaceStatusToken(thisNs.status)}
                  title={thisNs.status}
                />
                <span className="text-muted">{thisNs.status}</span>
              </>
            ) : null}
          </span>
        }
        actions={
          <Button
            variant="primary"
            loading={triggerBatch.isPending}
            onClick={() => triggerBatch.mutate(name)}
          >
            Run batch now
          </Button>
        }
      />

      {triggerBatch.isError ? (
        <Notice tone="fail" title="Trigger failed">
          {(triggerBatch.error as Error)?.message ??
            'Unable to start a batch run.'}
        </Notice>
      ) : null}

      {triggerBatch.isSuccess && triggerBatch.data ? (
        <Notice tone="ok" title={`Batch run #${triggerBatch.data.id} queued`}>
          The overview panels will refresh as the run lands; this notice goes
          away on next navigation.
        </Notice>
      ) : null}

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        {/* ─── HEALTH ─── */}
        <Panel title="health">
          {health.isLoading ? (
            <LoadingState rows={3} label="loading health" />
          ) : health.isError ? (
            <Notice tone="fail" title="Failed to load health">
              {(health.error as Error)?.message ?? 'Health probe failed.'}
            </Notice>
          ) : healthData ? (
            <ul className="flex flex-col gap-2 text-sm">
              {(['postgres', 'redis', 'qdrant'] as const).map((p) => (
                <li key={p} className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <StatusToken
                      state={probeState(healthData[p])}
                      title={healthData[p]}
                    />
                    <span className="font-mono text-primary">{p}</span>
                  </div>
                  <span className="font-mono text-xs text-muted">
                    {healthData[p]}
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
        </Panel>

        {/* ─── LAST BATCH RUN ─── */}
        <Panel title="last batch run">
          {overview.isLoading ? (
            <LoadingState rows={5} label="loading last batch run" />
          ) : lastRun ? (
            <KeyValueList
              rows={[
                {
                  label: 'status',
                  value: (
                    <StatusToken
                      state={lastRunToken(lastRun)}
                      title={
                        lastRun.success
                          ? 'success'
                          : lastRun.error_message ?? 'failed'
                      }
                      label={`#${lastRun.id}`}
                    />
                  ),
                },
                { label: 'started', value: formatTimestamp(lastRun.started_at) },
                { label: 'duration', value: formatDurationMs(lastRun.duration_ms) },
                {
                  label: 'entities',
                  value: formatNumber(lastRun.entities_processed),
                },
                { label: 'trigger', value: lastRun.trigger_source },
                {
                  label: 'phase 1 sparse',
                  value: (
                    <StatusToken
                      state={phaseToken(lastRun.phase1_ok)}
                      label={formatDurationMs(lastRun.phase1_duration_ms)}
                    />
                  ),
                },
                {
                  label: 'phase 2 dense',
                  value: (
                    <StatusToken
                      state={phaseToken(lastRun.phase2_ok)}
                      label={formatDurationMs(lastRun.phase2_duration_ms)}
                    />
                  ),
                },
                {
                  label: 'phase 3 trending',
                  value: (
                    <StatusToken
                      state={phaseToken(lastRun.phase3_ok)}
                      label={formatDurationMs(lastRun.phase3_duration_ms)}
                    />
                  ),
                },
              ]}
            />
          ) : (
            <EmptyState
              title="No batch runs yet"
              description="Trigger a manual run with the button above or wait for the next cron cycle."
            />
          )}
        </Panel>

        {/* ─── VOLUME (24h) ─── */}
        <Panel title="volume (24h)">
          {overview.isLoading || catalog.isLoading ? (
            <LoadingState rows={2} label="loading volume" />
          ) : (
            <div className="grid grid-cols-2 gap-3">
              <MetricTile
                label="events"
                value={formatNumber(thisNs?.active_events_24h ?? 0)}
                hint="last 24h"
              />
              <MetricTile
                label="dead-letter"
                value={formatNumber(catalog.data?.backlog.dead_letter ?? 0)}
                hint="catalog embed queue"
              />
            </div>
          )}
        </Panel>

        {/* ─── EMBEDDING ─── */}
        <Panel title="embedding">
          {config.isLoading ? (
            <LoadingState rows={4} label="loading embedding config" />
          ) : config.isError || !config.data ? (
            <Notice tone="fail" title="Failed to load embedding config">
              {(config.error as Error)?.message ??
                'Unable to read namespace config.'}
            </Notice>
          ) : (
            <KeyValueList
              rows={[
                { label: 'strategy', value: config.data.dense_strategy },
                { label: 'dim', value: config.data.embedding_dim.toString() },
                { label: 'distance', value: config.data.dense_distance },
                { label: 'alpha', value: config.data.alpha.toFixed(2) },
                {
                  label: 'catalog auto-embed',
                  value: catalog.data ? (
                    <StatusToken
                      state={catalog.data.catalog.enabled ? 'ok' : 'idle'}
                      label={catalog.data.catalog.enabled ? 'enabled' : 'disabled'}
                    />
                  ) : (
                    '—'
                  ),
                },
                {
                  label: 'catalog backlog',
                  value: catalog.data
                    ? formatNumber(catalog.data.backlog.pending + catalog.data.backlog.in_flight)
                    : '—',
                },
              ]}
            />
          )}
        </Panel>
      </div>

      {/* ─── TRENDING TOP 5 ─── */}
      <Panel
        title="trending top 5"
        actions={
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate(paths.nsTrending(name))}
          >
            view all trending
          </Button>
        }
      >
        {trending.isLoading ? (
          <LoadingState rows={5} label="loading trending" />
        ) : trending.isError ? (
          <Notice tone="fail" title="Failed to load trending">
            {(trending.error as Error)?.message ?? 'Unable to load trending items.'}
          </Notice>
        ) : !trending.data || trending.data.items.length === 0 ? (
          <EmptyState
            title="No trending items yet"
            description="Ingest events and let the cron job populate the trending ZSET; this panel updates on the next refresh."
          />
        ) : (
          <Table>
            <Thead>
              <Tr>
                <Th align="right">rank</Th>
                <Th>object_id</Th>
                <Th align="right">score</Th>
              </Tr>
            </Thead>
            <Tbody>
              {trending.data.items.map((item, idx) => (
                <Tr key={item.object_id}>
                  <Td mono align="right">{idx + 1}</Td>
                  <Td mono>
                    <Link to={paths.nsTrending(name)} className="hover:text-accent">
                      {item.object_id}
                    </Link>
                  </Td>
                  <Td mono align="right">{item.score.toFixed(1)}</Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </Panel>
    </PageShell>
  )
}
