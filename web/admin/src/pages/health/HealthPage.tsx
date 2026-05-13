import {
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  Panel,
  StatusToken,
  useRegisterCommand,
} from '../../components/ui'
import { probeState, useHealth } from '../../services/health'

interface ProbeRow {
  name: string
  value: string
}

export default function HealthPage() {
  const { data, isLoading, isError, error, refetch, isFetching } = useHealth()

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
        { name: 'postgres', value: data.postgres },
        { name: 'redis',    value: data.redis },
        { name: 'qdrant',   value: data.qdrant },
      ]
    : []

  const overall = data ? probeState(data.status) : 'ok'

  return (
    <PageShell>
      <PageHeader
        title="Health"
        meta={
          data ? (
            <>
              overall: <span className="text-primary">{data.status}</span>
            </>
          ) : null
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

      <Panel title="Probes">
        {isLoading ? (
          <LoadingState rows={3} />
        ) : (
          <ul className="flex flex-col gap-2">
            {probes.map((p) => (
              <li
                key={p.name}
                className="flex items-center justify-between text-sm"
              >
                <div className="flex items-center gap-3">
                  <StatusToken state={probeState(p.value)} title={p.value} />
                  <span className="font-mono text-primary">{p.name}</span>
                </div>
                <span className="font-mono tabular-nums text-xs text-muted">
                  {p.value}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Panel>
    </PageShell>
  )
}
