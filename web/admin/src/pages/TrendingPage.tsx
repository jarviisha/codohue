import { useState } from 'react'
import { useTrending } from '../hooks/useTrending'
import { useNamespaceList } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

function formatTTL(ttlSec: number): string {
  if (ttlSec === -2) return 'no cache'
  if (ttlSec === -1) return 'no expiry'
  const m = Math.floor(ttlSec / 60)
  const s = ttlSec % 60
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

export default function TrendingPage() {
  const { data: nsData } = useNamespaceList()
  const [namespace, setNamespace] = useState('')

  const { data, error, isLoading } = useTrending(namespace)

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '1rem' }}>
        <h2 style={{ margin: 0 }}>Trending Items</h2>
        <select value={namespace} onChange={e => setNamespace(e.target.value)} style={{ padding: '0.4rem 0.6rem', border: '1px solid #ccc', borderRadius: 4 }}>
          <option value="">Select namespace</option>
          {nsData?.namespaces.map(ns => (
            <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
          ))}
        </select>
      </div>

      {!namespace && <p style={{ color: '#888' }}>Select a namespace to view trending items.</p>}
      {error && <ErrorBanner message="Failed to load trending data." />}
      {isLoading && <p style={{ color: '#888' }}>Loading…</p>}

      {data && (
        <>
          <div style={{ marginBottom: '0.75rem', display: 'flex', gap: '1.5rem', fontSize: '0.85rem', color: '#555' }}>
            <span>Window: <strong>{data.window_hours}h</strong></span>
            <span>Total: <strong>{data.total}</strong></span>
            <span style={{ color: data.cache_ttl_sec === -2 ? '#ea4335' : '#34a853' }}>
              Cache: <strong>{formatTTL(data.cache_ttl_sec)}</strong>
            </span>
          </div>

          {data.cache_ttl_sec === -2 ? (
            <div style={{ background: '#fff', border: '1px solid #e0e0e0', borderRadius: 8, padding: '2rem', textAlign: 'center', color: '#888' }}>
              No trending data — run <code>make run-cron</code> to populate the trending cache.
            </div>
          ) : data.items.length === 0 ? (
            <p style={{ color: '#888' }}>No items in trending window.</p>
          ) : (
            <div style={{ background: '#fff', border: '1px solid #e0e0e0', borderRadius: 8, overflow: 'hidden' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ background: '#f8f9fa', borderBottom: '1px solid #e0e0e0' }}>
                    <th style={th}>#</th>
                    <th style={th}>Object ID</th>
                    <th style={th}>Score</th>
                    <th style={th}>Cache TTL</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((item, i) => (
                    <tr key={item.object_id} style={{ borderBottom: '1px solid #f0f0f0' }}>
                      <td style={td}>{i + 1}</td>
                      <td style={td}><code>{item.object_id}</code></td>
                      <td style={td}>{item.score.toFixed(2)}</td>
                      <td style={td}>{formatTTL(item.cache_ttl_sec)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  )
}

const th: React.CSSProperties = { padding: '0.65rem 1rem', textAlign: 'left', fontSize: '0.85rem', fontWeight: 600, color: '#666' }
const td: React.CSSProperties = { padding: '0.65rem 1rem', fontSize: '0.9rem' }
