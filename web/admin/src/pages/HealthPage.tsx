import { useHealth } from '../hooks/useHealth'
import StatusCard from '../components/StatusCard'
import ErrorBanner from '../components/ErrorBanner'

export default function HealthPage() {
  const { data, error, isLoading, dataUpdatedAt } = useHealth()

  const overallOk = data?.status === 'ok'
  const overallDegraded = data?.status === 'degraded'

  return (
    <div>
      <div className="mb-6">
        <h2
          className="font-light text-[#061b31] m-0"
          style={{ fontSize: '26px', letterSpacing: '-0.26px', lineHeight: 1.12 }}
        >
          System Health
        </h2>
      </div>

      {error && <ErrorBanner message="Could not reach the API server. Check that cmd/api is running." />}
      {isLoading && <p className="text-sm text-[#64748d] font-light">Checking health…</p>}

      {data && (
        <>
          <div
            className="inline-flex items-center gap-2 px-4 py-2 mb-6 text-sm font-normal"
            style={{
              border: `1px solid ${overallOk ? 'rgba(21,190,83,0.3)' : overallDegraded ? 'rgba(245,158,11,0.3)' : 'rgba(234,34,97,0.25)'}`,
              background: overallOk ? 'rgba(21,190,83,0.06)' : overallDegraded ? 'rgba(245,158,11,0.06)' : 'rgba(234,34,97,0.05)',
              borderRadius: '4px',
              color: overallOk ? '#108c3d' : overallDegraded ? '#92400e' : '#ea2261',
            }}
          >
            <span
              className="w-2 h-2 rounded-full shrink-0"
              style={{ background: overallOk ? '#15be53' : overallDegraded ? '#f59e0b' : '#ea2261' }}
              aria-hidden="true"
            />
            <span>Overall status: <strong className="font-normal">{data.status}</strong></span>
            {dataUpdatedAt > 0 && (
              <span className="text-xs text-[#64748d] ml-2 font-light">
                checked {new Date(dataUpdatedAt).toLocaleTimeString()}
              </span>
            )}
          </div>

          <div className="flex gap-3 flex-wrap">
            <StatusCard name="PostgreSQL" status={data.postgres} />
            <StatusCard name="Redis" status={data.redis} />
            <StatusCard name="Qdrant" status={data.qdrant} />
          </div>
        </>
      )}
    </div>
  )
}
