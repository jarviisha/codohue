import { useHealth } from '../hooks/useHealth'
import StatusCard from '../components/StatusCard'
import ErrorBanner from '../components/ErrorBanner'
import { PageHeader } from '../components/ui'

export default function HealthPage() {
  const { data, error, isLoading, dataUpdatedAt } = useHealth()

  const overallOk = data?.status === 'ok'
  const overallDegraded = data?.status === 'degraded'

  const statusBadgeClass = overallOk
    ? 'bg-success-bg border border-success/30 text-success'
    : overallDegraded
      ? 'bg-warning-bg border border-warning/30 text-warning'
      : 'bg-danger-bg border border-danger/25 text-danger'

  const statusDotClass = overallOk ? 'bg-success' : overallDegraded ? 'bg-warning' : 'bg-danger'

  return (
    <div>
      <PageHeader title="System Health" />

      {error && <ErrorBanner message="Could not reach the admin server." />}
      {isLoading && <p className="text-sm text-muted">Checking health…</p>}

      {data && (
        <>
          {data.status === 'error' && (
            <ErrorBanner message="Could not reach the API server. Check that cmd/api is running." />
          )}

          <div className={`inline-flex items-center gap-2 px-4 py-2 mb-6 text-sm font-medium rounded-lg ${statusBadgeClass}`}>
            <span className={`w-2 h-2 rounded-full shrink-0 ${statusDotClass}`} aria-hidden="true" />
            <span>
              Overall status: <strong className="font-semibold">{data.status}</strong>
            </span>
            {dataUpdatedAt > 0 && (
              <span className="text-xs text-muted ml-2">
                checked {new Date(dataUpdatedAt).toLocaleTimeString()}
              </span>
            )}
          </div>

          {data.status !== 'error' && (
            <div className="flex gap-3 flex-wrap">
              <StatusCard name="PostgreSQL" status={data.postgres} />
              <StatusCard name="Redis" status={data.redis} />
              <StatusCard name="Qdrant" status={data.qdrant} />
            </div>
          )}
        </>
      )}
    </div>
  )
}
