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
      <div className="flex items-center gap-4 mb-4">
        <h2 className="m-0 text-xl font-semibold text-gray-800">Trending Items</h2>
        <select
          value={namespace}
          onChange={e => setNamespace(e.target.value)}
          className="px-2.5 py-1.5 border border-gray-300 rounded text-sm"
        >
          <option value="">Select namespace</option>
          {nsData?.namespaces.map(ns => (
            <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
          ))}
        </select>
      </div>

      {!namespace && <p className="text-gray-400">Select a namespace to view trending items.</p>}
      {error && <ErrorBanner message="Failed to load trending data." />}
      {isLoading && <p className="text-gray-400">Loading…</p>}

      {data && (
        <>
          <div className="mb-3 flex gap-6 text-sm text-gray-600">
            <span>Window: <strong className="text-gray-800">{data.window_hours}h</strong></span>
            <span>Total: <strong className="text-gray-800">{data.total}</strong></span>
            <span className={data.cache_ttl_sec === -2 ? 'text-red-500' : 'text-green-600'}>
              Cache: <strong>{formatTTL(data.cache_ttl_sec)}</strong>
            </span>
          </div>

          {data.cache_ttl_sec === -2 ? (
            <div className="bg-white border border-gray-200 rounded-lg p-8 text-center text-gray-400">
              No trending data — run <code className="font-mono text-sm bg-gray-100 px-1.5 py-0.5 rounded">make run-cron</code> to populate the trending cache.
            </div>
          ) : data.items.length === 0 ? (
            <p className="text-gray-400">No items in trending window.</p>
          ) : (
            <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
              <table className="w-full border-collapse">
                <thead>
                  <tr className="bg-gray-50 border-b border-gray-200">
                    <th className={th}>#</th>
                    <th className={th}>Object ID</th>
                    <th className={th}>Score</th>
                    <th className={th}>Cache TTL</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((item, i) => (
                    <tr key={item.object_id} className="border-b border-gray-100">
                      <td className={td}>{i + 1}</td>
                      <td className={td}><code className="font-mono text-sm bg-gray-100 px-1.5 py-0.5 rounded">{item.object_id}</code></td>
                      <td className={td}>{item.score.toFixed(2)}</td>
                      <td className={td}>{formatTTL(item.cache_ttl_sec)}</td>
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

const th = 'px-4 py-2.5 text-left text-sm font-semibold text-gray-500'
const td = 'px-4 py-2.5 text-sm text-gray-700'
