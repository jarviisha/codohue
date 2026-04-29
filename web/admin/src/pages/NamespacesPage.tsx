import { useNavigate } from 'react-router-dom'
import { useNamespaceList } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

export default function NamespacesPage() {
  const { data, error, isLoading } = useNamespaceList()
  const navigate = useNavigate()

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
        <h2 style={{ margin: 0 }}>Namespaces</h2>
        <button
          onClick={() => navigate('/namespaces/new')}
          style={{ padding: '0.5rem 1rem', background: '#1a73e8', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
        >
          + Create Namespace
        </button>
      </div>

      {error && <ErrorBanner message="Failed to load namespaces." />}
      {isLoading && <p style={{ color: '#888' }}>Loading…</p>}

      {data && (
        <div style={{ background: '#fff', border: '1px solid #e0e0e0', borderRadius: 8, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ background: '#f8f9fa', borderBottom: '1px solid #e0e0e0' }}>
                <th style={th}>Namespace</th>
                <th style={th}>Strategy</th>
                <th style={th}>Max Results</th>
                <th style={th}>API Key</th>
                <th style={th}>Updated</th>
                <th style={th}></th>
              </tr>
            </thead>
            <tbody>
              {data.namespaces.length === 0 && (
                <tr><td colSpan={6} style={{ padding: '1rem', textAlign: 'center', color: '#888' }}>No namespaces yet</td></tr>
              )}
              {data.namespaces.map(ns => (
                <tr key={ns.namespace} style={{ borderBottom: '1px solid #f0f0f0' }}>
                  <td style={td}><code>{ns.namespace}</code></td>
                  <td style={td}>{ns.dense_strategy}</td>
                  <td style={td}>{ns.max_results}</td>
                  <td style={td}>{ns.has_api_key ? '✓' : '–'}</td>
                  <td style={td}>{new Date(ns.updated_at).toLocaleString()}</td>
                  <td style={td}>
                    <button onClick={() => navigate(`/namespaces/${ns.namespace}`)} style={{ background: 'none', border: '1px solid #ccc', borderRadius: 4, cursor: 'pointer', padding: '0.25rem 0.5rem', fontSize: '0.85rem' }}>
                      Edit
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

const th: React.CSSProperties = { padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.85rem', fontWeight: 600, color: '#666' }
const td: React.CSSProperties = { padding: '0.75rem 1rem', fontSize: '0.9rem' }
