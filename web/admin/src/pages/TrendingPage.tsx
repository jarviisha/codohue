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
    <span className={`text-xs font-semibold uppercase tracking-[0.04em] px-2 py-0.5 rounded-sm tabular-nums ${missing ? 'bg-danger-bg border border-danger/25 text-danger' : 'bg-success-bg border border-success/30 text-success'}`}>
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
      <div className="flex items-center gap-4 mb-8">
        <h2 className="text-[28px] font-semibold text-primary -tracking-[0.01em] leading-tight m-0">
          Trending Items
        </h2>
        <select
          value={namespace}
          onChange={e => setNamespace(e.target.value)}
          className="bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary text-sm px-3 py-2 rounded-md focus:outline-none transition-shadow duration-100"
        >
          <option value="">Select namespace</option>
          {nsData?.namespaces.map(ns => (
            <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
          ))}
        </select>
      </div>

      {!namespace && <p className="text-sm text-muted">Select a namespace to view trending items.</p>}
      {error && <ErrorBanner message="Failed to load trending data." />}
      {isLoading && <p className="text-sm text-muted">Loading…</p>}

      {data && (
        <>
          <div className="flex gap-4 mb-6 flex-wrap">
            <div className="flex flex-col px-4 py-3 bg-surface border border-default rounded-lg min-w-[100px]">
              <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted mb-1">Window</span>
              <span className="text-sm font-semibold text-primary tabular-nums">{data.window_hours}h</span>
            </div>
            <div className="flex flex-col px-4 py-3 bg-surface border border-default rounded-lg min-w-[100px]">
              <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted mb-1">Total items</span>
              <span className="text-sm font-semibold text-primary tabular-nums">{data.total}</span>
            </div>
            <div className="flex flex-col px-4 py-3 bg-surface border border-default rounded-lg min-w-[100px]">
              <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted mb-1">Cache TTL</span>
              <TTLBadge ttl={data.cache_ttl_sec} />
            </div>
          </div>

          {data.cache_ttl_sec === -2 ? (
            <div className="p-10 text-center text-sm text-muted border border-dashed border-default rounded-lg">
              No trending data — run{' '}
              <code className="font-mono text-[12px] bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm">
                make run-cron
              </code>{' '}
              to populate the trending cache.
            </div>
          ) : data.items.length === 0 ? (
            <p className="text-sm text-muted">No items in trending window.</p>
          ) : (
            <div className="bg-surface border border-default rounded-lg overflow-hidden">
              <table className="w-full border-collapse">
                <thead>
                  <tr className="bg-subtle border-b-2 border-default">
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">#</th>
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Object ID</th>
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Score</th>
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Cache TTL</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((item, i) => (
                    <tr key={item.object_id} className="border-b border-default hover:bg-surface-raised">
                      <td className="px-4 py-3 text-sm text-muted tabular-nums">{i + 1}</td>
                      <td className="px-4 py-3 text-sm">
                        <code className="font-mono text-[12px] bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm font-medium">
                          {item.object_id}
                        </code>
                      </td>
                      <td className="px-4 py-3 text-sm text-primary font-mono tabular-nums">
                        {item.score.toFixed(2)}
                      </td>
                      <td className="px-4 py-3 text-sm">
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
