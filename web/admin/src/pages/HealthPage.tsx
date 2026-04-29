import { useHealth } from '../hooks/useHealth'
import StatusCard from '../components/StatusCard'
import ErrorBanner from '../components/ErrorBanner'

export default function HealthPage() {
  const { data, error, isLoading, dataUpdatedAt } = useHealth()

  const overallColor = data?.status === 'ok' ? '#34a853' : data?.status === 'degraded' ? '#fbbc04' : '#ea4335'

  return (
    <div>
      <h2 className="mt-0 mb-4 text-xl font-semibold text-gray-800">System Health</h2>

      {error && <ErrorBanner message="Could not reach the API server. Check that cmd/api is running." />}

      {data && (
        <div
          className="bg-white px-4 py-3 rounded-lg mb-6 inline-flex items-center gap-2 border-2"
          style={{ borderColor: overallColor }}
        >
          <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: overallColor }} aria-hidden="true" />
          <strong className="text-gray-800">Overall: {data.status}</strong>
          {dataUpdatedAt > 0 && (
            <span className="text-xs text-gray-400 ml-2">
              Last checked {new Date(dataUpdatedAt).toLocaleTimeString()}
            </span>
          )}
        </div>
      )}

      {isLoading && <p className="text-gray-400">Checking health…</p>}

      {data && (
        <div className="flex gap-4 flex-wrap">
          <StatusCard name="PostgreSQL" status={data.postgres} />
          <StatusCard name="Redis" status={data.redis} />
          <StatusCard name="Qdrant" status={data.qdrant} />
        </div>
      )}
    </div>
  )
}
