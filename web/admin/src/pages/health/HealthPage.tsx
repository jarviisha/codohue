import {
  Button,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  Panel,
  StatusToken,
  useRegisterCommand,
} from '@/components/ui'
import { probeState, useHealth } from '@/services/health'

interface ProbeRow {
  name: string
  value: string
  description: string
}

export default function HealthPage() {
  const {
    data,
    dataUpdatedAt,
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
  } = useHealth()

  useRegisterCommand(
    'health.refresh',
    'Refresh health probes',
    () => {
      if (!isFetching) void refetch()
    },
    'global',
  )

  const probes: ProbeRow[] = data
    ? [
        {
          name: 'postgres',
          value: data.postgres,
          description: 'durable config, events, batch logs',
        },
        {
          name: 'redis',
          value: data.redis,
          description: 'streams, trending cache, worker state',
        },
        {
          name: 'qdrant',
          value: data.qdrant,
          description: 'sparse and dense vector collections',
        },
      ]
    : []

  const overall = data ? probeState(data.status) : 'ok'
  const healthyCount = probes.filter((p) => probeState(p.value) === 'ok').length
  const checkedAt = dataUpdatedAt
    ? new Date(dataUpdatedAt).toLocaleTimeString('en-US', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      })
    : '—'

  return (
    <PageShell>
      <PageHeader
        title="health"
        meta={
          data ? (
            <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <StatusToken state={overall} title={data.status} />
              <span>
                admin plane{' '}
                <span className="font-mono text-primary">{data.status}</span>
              </span>
              <span aria-hidden>·</span>
              <span>
                {healthyCount}/{probes.length} probes healthy
              </span>
              <span aria-hidden>·</span>
              <span className="font-mono tabular-nums">
                checked {checkedAt}
              </span>
            </span>
          ) : (
            'loading health probes'
          )
        }
        actions={
          <Button
            variant="secondary"
            size="sm"
            loading={isFetching && !isLoading}
            onClick={() => void refetch()}
          >
            Refresh
          </Button>
        }
      />

      {isError ? (
        <Notice tone="fail" title="Health check failed">
          {(error as Error)?.message ?? 'Unable to reach the admin API.'}
        </Notice>
      ) : null}

      {data && overall !== 'ok' ? (
        <Notice tone="warn" title="Service degraded">
          One or more probes are reporting a non-ok status.
        </Notice>
      ) : null}

      {isLoading ? (
        <Panel title="health probes">
          <LoadingState rows={4} />
        </Panel>
      ) : data ? (
        <>
          <Panel title="summary">
            <div className="grid grid-cols-1 md:grid-cols-[1fr_auto] gap-4 md:items-center">
              <div className="flex items-center gap-3">
                <StatusToken state={overall} title={data.status} />
                <div>
                  <div className="font-mono text-primary">
                    admin plane {data.status}
                  </div>
                  <div className="text-sm text-muted leading-5">
                    {healthyCount} of {probes.length} probes healthy · refreshes
                    every 30s
                  </div>
                </div>
              </div>
              <div className="font-mono text-sm tabular-nums text-secondary">
                checked {checkedAt}
              </div>
            </div>
          </Panel>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {probes.map((p) => {
              const state = probeState(p.value)
              return (
                <section
                  key={p.name}
                  className="bg-surface border border-default rounded-sm p-5 flex flex-col gap-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <h2 className="font-mono text-sm text-primary">
                        {p.name}
                      </h2>
                      <p className="text-sm text-muted leading-5 mt-1">
                        {p.description}
                      </p>
                    </div>
                    <StatusToken state={state} title={p.value} />
                  </div>
                  <div className="border-t border-default pt-3">
                    <div className="font-mono text-xs uppercase tracking-[0.04em] text-secondary">
                      reported status
                    </div>
                    <div className="mt-1 font-mono text-lg tabular-nums text-primary">
                      {p.value}
                    </div>
                  </div>
                </section>
              )
            })}
          </div>
        </>
      ) : null}
    </PageShell>
  )
}
