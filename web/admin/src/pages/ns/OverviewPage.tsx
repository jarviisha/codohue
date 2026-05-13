import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
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
  useRegisterCommand,
} from '../../components/ui'
import type { StatusState } from '../../components/ui'
import { http } from '../../services/http'
import { probeState, useHealth } from '../../services/health'
import {
  lastRunToken,
  namespaceKeys,
  namespaceStatusToken,
  useNamespace,
  useNamespacesOverview,
} from '../../services/namespaces'
import { paths } from '../../routes/path'
import {
  formatDurationMs,
  formatNumber,
  formatTimestamp,
} from '../../utils/format'

function phaseToken(ok: boolean | null | undefined): StatusState {
  if (ok === null || ok === undefined) return 'idle'
  return ok ? 'ok' : 'fail'
}

interface BatchRunCreateResponse {
  id: number
  namespace: string
}

export default function NamespaceOverviewPage() {
  const { name = '' } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const qc = useQueryClient()

  const health = useHealth()
  const overview = useNamespacesOverview()
  const config = useNamespace(name)

  // Inline batch-run trigger; services/batchRuns.ts (Phase 2.D.1) will move
  // this into a proper hook. Kept here so Overview can ship its primary
  // action without depending on a domain that hasn't landed yet.
  const triggerBatch = useMutation({
    mutationFn: () =>
      http.post<BatchRunCreateResponse>(
        `/api/admin/v1/namespaces/${encodeURIComponent(name)}/batch-runs`,
        {},
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: namespaceKeys.overview() })
      qc.invalidateQueries({ queryKey: namespaceKeys.byName(name) })
    },
  })

  useRegisterCommand(
    `ns.${name}.batch.run`,
    `Run batch on ${name}`,
    () => {
      if (!triggerBatch.isPending) triggerBatch.mutate()
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
            onClick={() => triggerBatch.mutate()}
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
            <LoadingState rows={3} />
          ) : health.isError ? (
            <Notice tone="fail">
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
            <LoadingState rows={5} />
          ) : lastRun ? (
            <KeyValueList
              rows={[
                {
                  label: 'status',
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <StatusToken
                        state={lastRunToken(lastRun)}
                        title={
                          lastRun.success
                            ? 'success'
                            : lastRun.error_message ?? 'failed'
                        }
                      />
                      <span>#{lastRun.id}</span>
                    </span>
                  ),
                },
                { label: 'started', value: formatTimestamp(lastRun.started_at) },
                { label: 'duration', value: formatDurationMs(lastRun.duration_ms) },
                {
                  label: 'subjects',
                  value: formatNumber(lastRun.subjects_processed),
                },
                { label: 'trigger', value: lastRun.trigger_source },
                {
                  label: 'phase 1 sparse',
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <StatusToken state={phaseToken(lastRun.phase1_ok)} />
                      <span>{formatDurationMs(lastRun.phase1_duration_ms)}</span>
                    </span>
                  ),
                },
                {
                  label: 'phase 2 dense',
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <StatusToken state={phaseToken(lastRun.phase2_ok)} />
                      <span>{formatDurationMs(lastRun.phase2_duration_ms)}</span>
                    </span>
                  ),
                },
                {
                  label: 'phase 3 trending',
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <StatusToken state={phaseToken(lastRun.phase3_ok)} />
                      <span>{formatDurationMs(lastRun.phase3_duration_ms)}</span>
                    </span>
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
          {overview.isLoading ? (
            <LoadingState rows={2} />
          ) : (
            <div className="grid grid-cols-2 gap-3">
              <MetricTile
                label="events"
                value={formatNumber(thisNs?.active_events_24h ?? 0)}
                hint="last 24h"
              />
              <MetricTile
                label="dead-letter"
                value="—"
                hint="wires in Phase 2.B catalog"
              />
            </div>
          )}
        </Panel>

        {/* ─── EMBEDDING ─── */}
        <Panel title="embedding">
          {config.isLoading ? (
            <LoadingState rows={4} />
          ) : config.isError || !config.data ? (
            <Notice tone="fail">
              {(config.error as Error)?.message ??
                'Failed to load namespace config.'}
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
                  value: (
                    <span className="text-muted">[PEND] wires in 2.B</span>
                  ),
                },
                {
                  label: 'catalog backlog',
                  value: (
                    <span className="text-muted">[PEND] wires in 2.B</span>
                  ),
                },
              ]}
            />
          )}
        </Panel>
      </div>

      {/* ─── TRENDING TOP 5 (placeholder until Phase 2.E.2) ─── */}
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
        <EmptyState
          title="Trending wiring lands in Phase 2.E"
          description="Once services/trending.ts and the trending page ship, this panel will render the top-5 items here."
        />
      </Panel>
    </PageShell>
  )
}
