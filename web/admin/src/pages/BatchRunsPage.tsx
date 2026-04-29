import { useState } from 'react'
import { useBatchRuns } from '../hooks/useBatchRuns'
import { useNamespaceList } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

export default function BatchRunsPage() {
  const { data: nsData } = useNamespaceList()
  const [nsFilter, setNsFilter] = useState('')
  const { data, error, isLoading } = useBatchRuns(nsFilter || undefined)

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
        <h2 style={{ margin: 0 }}>Batch Runs</h2>
        <select value={nsFilter} onChange={e => setNsFilter(e.target.value)} style={{ padding: '0.4rem 0.6rem', border: '1px solid #ccc', borderRadius: 4 }}>
          <option value="">All namespaces</option>
          {nsData?.namespaces.map(ns => (
            <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
          ))}
        </select>
      </div>

      {error && <ErrorBanner message="Failed to load batch runs." />}
      {isLoading && <p style={{ color: '#888' }}>Loading…</p>}

      {data && data.runs.length === 0 && (
        <div style={{ background: '#fff', border: '1px solid #e0e0e0', borderRadius: 8, padding: '2rem', textAlign: 'center', color: '#888' }}>
          No runs yet — run <code>make run-cron</code> to populate batch history.
        </div>
      )}

      {data && data.runs.length > 0 && (
        <div style={{ background: '#fff', border: '1px solid #e0e0e0', borderRadius: 8, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ background: '#f8f9fa', borderBottom: '1px solid #e0e0e0' }}>
                <th style={th}>ID</th>
                <th style={th}>Namespace</th>
                <th style={th}>Started</th>
                <th style={th}>Duration</th>
                <th style={th}>Subjects</th>
                <th style={th}>Status</th>
              </tr>
            </thead>
            <tbody>
              {data.runs.map(run => (
                <tr key={run.id} style={{ borderBottom: '1px solid #f0f0f0' }}>
                  <td style={td}>{run.id}</td>
                  <td style={td}><code>{run.namespace}</code></td>
                  <td style={td}>{new Date(run.started_at).toLocaleString()}</td>
                  <td style={td}>{run.duration_ms != null ? `${run.duration_ms} ms` : run.completed_at ? '–' : <em style={{ color: '#888' }}>in progress</em>}</td>
                  <td style={td}>{run.subjects_processed}</td>
                  <td style={td}>
                    {run.success ? (
                      <span style={{ color: '#34a853', fontWeight: 600 }}>✓ OK</span>
                    ) : run.completed_at ? (
                      <details>
                        <summary style={{ cursor: 'pointer', color: '#ea4335', fontWeight: 600 }}>✗ Failed</summary>
                        <pre style={{ margin: '0.25rem 0 0', fontSize: '0.8rem', color: '#c00', whiteSpace: 'pre-wrap' }}>{run.error_message}</pre>
                      </details>
                    ) : (
                      <span style={{ color: '#fbbc04' }}>⟳ Running</span>
                    )}
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

const th: React.CSSProperties = { padding: '0.65rem 1rem', textAlign: 'left', fontSize: '0.85rem', fontWeight: 600, color: '#666' }
const td: React.CSSProperties = { padding: '0.65rem 1rem', fontSize: '0.9rem' }
