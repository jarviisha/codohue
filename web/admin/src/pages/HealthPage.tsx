import { useHealth } from '../hooks/useHealth'
import StatusCard from '../components/StatusCard'
import ErrorBanner from '../components/ErrorBanner'
import { Badge, LoadingState, PageHeader, PageShell } from '../components/ui'
import { formatTimeOfDay } from '../utils/format'

export default function HealthPage() {
  const { data, error, isLoading, dataUpdatedAt } = useHealth()

  const overallOk = data?.status === 'ok'
  const overallDegraded = data?.status === 'degraded'

  const statusTone = overallOk ? 'success' : overallDegraded ? 'warning' : 'danger'

  return (
    <PageShell>
      <PageHeader title="System Health" />

      {error && <ErrorBanner message="Could not reach the admin server." />}
      {isLoading && <LoadingState label="Checking health..." />}

      {data && (
        <>
          {data.status === 'error' && (
            <ErrorBanner message="Could not reach the API server. Check that cmd/api is running." />
          )}

          <div>
            <Badge tone={statusTone} size="md" dot className="normal-case tracking-normal">
              Overall status: <strong className="font-semibold">{data.status}</strong>
              {dataUpdatedAt > 0 && (
                <span className="ml-2 text-xs text-muted normal-nums">
                  checked {formatTimeOfDay(dataUpdatedAt)}
                </span>
              )}
            </Badge>
          </div>

          {data.status !== 'error' && (
            <div className="flex flex-wrap gap-3">
              <StatusCard name="PostgreSQL" status={data.postgres} />
              <StatusCard name="Redis" status={data.redis} />
              <StatusCard name="Qdrant" status={data.qdrant} />
            </div>
          )}
        </>
      )}
    </PageShell>
  )
}
