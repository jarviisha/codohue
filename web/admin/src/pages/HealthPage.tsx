import { useHealth } from '../hooks/useHealth'
import StatusCard from '../components/StatusCard'
import ErrorBanner from '../components/ErrorBanner'

export default function HealthPage() {
  const { data, error, isLoading, dataUpdatedAt } = useHealth()

  const overallColor = data?.status === 'ok' ? '#34a853' : data?.status === 'degraded' ? '#fbbc04' : '#ea4335'

  return (
    <div>
      <h2 style={{ marginTop: 0 }}>System Health</h2>

      {error && <ErrorBanner message="Could not reach the API server. Check that cmd/api is running." />}

      {data && (
        <div style={{ background: '#fff', border: `2px solid ${overallColor}`, borderRadius: 8, padding: '0.75rem 1rem', marginBottom: '1.5rem', display: 'inline-flex', alignItems: 'center', gap: '0.5rem' }}>
          <span style={{ width: 10, height: 10, borderRadius: '50%', background: overallColor }} aria-hidden="true" />
          <strong>Overall: {data.status}</strong>
          {dataUpdatedAt > 0 && (
            <span style={{ fontSize: '0.8rem', color: '#888', marginLeft: '0.5rem' }}>
              Last checked {new Date(dataUpdatedAt).toLocaleTimeString()}
            </span>
          )}
        </div>
      )}

      {isLoading && <p style={{ color: '#888' }}>Checking health…</p>}

      {data && (
        <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
          <StatusCard name="PostgreSQL" status={data.postgres} />
          <StatusCard name="Redis" status={data.redis} />
          <StatusCard name="Qdrant" status={data.qdrant} />
        </div>
      )}
    </div>
  )
}
