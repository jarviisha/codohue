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

function TTLBadge({ ttl }: { ttl: number }) {
  const missing = ttl === -2
  return (
    <span
      className="text-xs font-normal px-2 py-0.5 tabular-nums"
      style={{
        background: missing ? 'rgba(234,34,97,0.06)' : 'rgba(21,190,83,0.08)',
        border: `1px solid ${missing ? 'rgba(234,34,97,0.2)' : 'rgba(21,190,83,0.3)'}`,
        borderRadius: '4px',
        color: missing ? '#ea2261' : '#108c3d',
      }}
    >
      {formatTTL(ttl)}
    </span>
  )
}

export default function TrendingPage() {
  const { data: nsData } = useNamespaceList()
  const [namespace, setNamespace] = useState('')
  const { data, error, isLoading } = useTrending(namespace)

  return (
    <div>
      <div className="flex items-center gap-4 mb-6">
        <h2
          className="font-light text-[#061b31] m-0"
          style={{ fontSize: '26px', letterSpacing: '-0.26px', lineHeight: 1.12 }}
        >
          Trending Items
        </h2>
        <select
          value={namespace}
          onChange={e => setNamespace(e.target.value)}
          className="text-sm font-normal outline-none"
          style={{
            padding: '6px 12px',
            border: '1px solid #e5edf5',
            borderRadius: '4px',
            color: '#061b31',
            background: '#fff',
          }}
          onFocus={e => { e.target.style.borderColor = '#533afd' }}
          onBlur={e => { e.target.style.borderColor = '#e5edf5' }}
        >
          <option value="">Select namespace</option>
          {nsData?.namespaces.map(ns => (
            <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
          ))}
        </select>
      </div>

      {!namespace && <p className="text-sm text-[#64748d] font-light">Select a namespace to view trending items.</p>}
      {error && <ErrorBanner message="Failed to load trending data." />}
      {isLoading && <p className="text-sm text-[#64748d] font-light">Loading…</p>}

      {data && (
        <>
          <div className="flex gap-5 mb-5 flex-wrap">
            <div
              className="flex flex-col px-4 py-3"
              style={{ border: '1px solid #e5edf5', borderRadius: '5px', minWidth: '100px' }}
            >
              <span className="text-xs text-[#64748d] font-light mb-0.5">Window</span>
              <span className="text-sm font-normal text-[#061b31] tabular-nums">{data.window_hours}h</span>
            </div>
            <div
              className="flex flex-col px-4 py-3"
              style={{ border: '1px solid #e5edf5', borderRadius: '5px', minWidth: '100px' }}
            >
              <span className="text-xs text-[#64748d] font-light mb-0.5">Total items</span>
              <span className="text-sm font-normal text-[#061b31] tabular-nums">{data.total}</span>
            </div>
            <div
              className="flex flex-col px-4 py-3"
              style={{ border: '1px solid #e5edf5', borderRadius: '5px', minWidth: '100px' }}
            >
              <span className="text-xs text-[#64748d] font-light mb-1">Cache TTL</span>
              <TTLBadge ttl={data.cache_ttl_sec} />
            </div>
          </div>

          {data.cache_ttl_sec === -2 ? (
            <div
              className="p-10 text-center text-sm text-[#64748d] font-light"
              style={{ border: '1px dashed #d6d9fc', borderRadius: '6px' }}
            >
              No trending data — run{' '}
              <code style={{ fontFamily: "'Source Code Pro', monospace", fontSize: '12px', background: '#f5f6ff', padding: '1px 6px', borderRadius: '3px', color: '#533afd' }}>
                make run-cron
              </code>{' '}
              to populate the trending cache.
            </div>
          ) : data.items.length === 0 ? (
            <p className="text-sm text-[#64748d] font-light">No items in trending window.</p>
          ) : (
            <div
              className="bg-white overflow-hidden"
              style={{ border: '1px solid #e5edf5', borderRadius: '6px', boxShadow: 'rgba(23,23,23,0.06) 0px 3px 6px' }}
            >
              <table className="w-full border-collapse">
                <thead>
                  <tr style={{ borderBottom: '1px solid #e5edf5' }}>
                    <th style={thStyle}>#</th>
                    <th style={thStyle}>Object ID</th>
                    <th style={thStyle}>Score</th>
                    <th style={thStyle}>Cache TTL</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((item, i) => (
                    <tr key={item.object_id} style={{ borderBottom: '1px solid #e5edf5' }}>
                      <td style={{ ...tdStyle, color: '#64748d' }} className="tabular-nums">{i + 1}</td>
                      <td style={tdStyle}>
                        <code style={{ fontFamily: "'Source Code Pro', monospace", fontSize: '12px', background: '#f5f6ff', padding: '1px 6px', borderRadius: '3px', color: '#533afd', fontWeight: 500 }}>
                          {item.object_id}
                        </code>
                      </td>
                      <td style={{ ...tdStyle, fontFamily: "'Source Code Pro', monospace", fontSize: '12px' }} className="tabular-nums">
                        {item.score.toFixed(2)}
                      </td>
                      <td style={tdStyle}>
                        <TTLBadge ttl={item.cache_ttl_sec} />
                      </td>
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

const thStyle: React.CSSProperties = {
  padding: '10px 16px',
  textAlign: 'left',
  fontSize: '12px',
  fontWeight: 400,
  color: '#64748d',
}

const tdStyle: React.CSSProperties = {
  padding: '10px 16px',
  fontSize: '13px',
  color: '#273951',
  fontWeight: 300,
}
