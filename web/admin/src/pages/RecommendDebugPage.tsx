import { useState } from 'react'
import { useRecommendDebug } from '../hooks/useRecommendDebug'
import { useNamespaceList } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

const LIMITS = [5, 10, 20, 50]

export default function RecommendDebugPage() {
  const { data: nsData } = useNamespaceList()
  const debug = useRecommendDebug()

  const [namespace, setNamespace] = useState('')
  const [subjectID, setSubjectID] = useState('')
  const [limit, setLimit] = useState(10)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    debug.mutate({ namespace, subject_id: subjectID, limit, offset: 0 })
  }

  return (
    <div>
      <h2 style={{ marginTop: 0 }}>Recommendation Debug</h2>

      <form onSubmit={handleSubmit} style={{ background: '#fff', border: '1px solid #e0e0e0', borderRadius: 8, padding: '1rem', marginBottom: '1.5rem', display: 'flex', gap: '0.75rem', flexWrap: 'wrap', alignItems: 'flex-end' }}>
        <div>
          <label style={labelStyle}>Namespace</label>
          <select required value={namespace} onChange={e => setNamespace(e.target.value)} style={inputStyle}>
            <option value="">Select namespace</option>
            {nsData?.namespaces.map(ns => (
              <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
            ))}
          </select>
        </div>
        <div>
          <label style={labelStyle}>Subject ID</label>
          <input required value={subjectID} onChange={e => setSubjectID(e.target.value)} placeholder="e.g. user-123" style={inputStyle} />
        </div>
        <div>
          <label style={labelStyle}>Limit</label>
          <select value={limit} onChange={e => setLimit(+e.target.value)} style={{ ...inputStyle, width: 80 }}>
            {LIMITS.map(l => <option key={l} value={l}>{l}</option>)}
          </select>
        </div>
        <button type="submit" disabled={debug.isPending} style={{ padding: '0.5rem 1rem', background: '#1a73e8', color: '#fff', border: 'none', borderRadius: 4, cursor: debug.isPending ? 'not-allowed' : 'pointer' }}>
          {debug.isPending ? 'Fetching…' : 'Fetch'}
        </button>
      </form>

      {debug.error && <ErrorBanner message={debug.error.message} />}

      {debug.data && (
        <div>
          <div style={{ marginBottom: '0.75rem', display: 'flex', gap: '1rem', fontSize: '0.85rem', color: '#555' }}>
            <span>Subject: <strong>{debug.data.subject_id}</strong></span>
            <span>Source: <strong style={{ background: '#e8f0fe', color: '#1a73e8', padding: '0.1rem 0.4rem', borderRadius: 3 }}>{debug.data.source}</strong></span>
            <span>Total: <strong>{debug.data.total}</strong></span>
          </div>

          {debug.data.items.length === 0 ? (
            <p style={{ color: '#888' }}>No recommendations found for this subject.</p>
          ) : (
            <div style={{ background: '#fff', border: '1px solid #e0e0e0', borderRadius: 8, overflow: 'hidden' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ background: '#f8f9fa', borderBottom: '1px solid #e0e0e0' }}>
                    <th style={th}>Rank</th>
                    <th style={th}>Object ID</th>
                    <th style={th}>Score</th>
                  </tr>
                </thead>
                <tbody>
                  {debug.data.items.map(item => (
                    <tr key={item.object_id} style={{ borderBottom: '1px solid #f0f0f0' }}>
                      <td style={td}>{item.rank}</td>
                      <td style={td}><code>{item.object_id}</code></td>
                      <td style={td}>{item.score.toFixed(4)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

const labelStyle: React.CSSProperties = { display: 'block', fontSize: '0.8rem', color: '#555', marginBottom: '0.25rem' }
const inputStyle: React.CSSProperties = { padding: '0.4rem 0.6rem', border: '1px solid #ccc', borderRadius: 4, fontSize: '0.9rem' }
const th: React.CSSProperties = { padding: '0.65rem 1rem', textAlign: 'left', fontSize: '0.85rem', fontWeight: 600, color: '#666' }
const td: React.CSSProperties = { padding: '0.65rem 1rem', fontSize: '0.9rem' }
