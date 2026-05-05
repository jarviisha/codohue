import { useTrending } from '../hooks/useTrending'
import ErrorBanner from '../components/ErrorBanner'
import { CodeBadge, EmptyState, PageHeader, Panel, Table, Thead, Th, Tbody, Tr, Td } from '../components/ui'
import { useActiveNamespace } from '../context/NamespaceContext'

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
    <span className={`text-xs font-semibold uppercase tracking-[0.04em] px-2 py-0.5 rounded-full tabular-nums ${missing ? 'bg-danger-bg border border-danger/25 text-danger' : 'bg-success-bg border border-success/30 text-success'}`}>
      {formatTTL(ttl)}
    </span>
  )
}

export default function TrendingPage() {
  const { namespace } = useActiveNamespace()
  const { data, error, isLoading } = useTrending(namespace)

  return (
    <div>
      <PageHeader title="Trending Items" />
      {error && <ErrorBanner message="Failed to load trending data." />}
      {isLoading && <p className="text-sm text-muted">Loading…</p>}

      {data && (
        <>
          <div className="flex gap-4 mb-6 flex-wrap">
            <div className="flex flex-col px-4 py-3 bg-surface border border-default rounded-xl min-w-24">
              <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted mb-1">Window</span>
              <span className="text-sm font-semibold text-primary tabular-nums">{data.window_hours}h</span>
            </div>
            <div className="flex flex-col px-4 py-3 bg-surface border border-default rounded-xl min-w-24">
              <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted mb-1">Total items</span>
              <span className="text-sm font-semibold text-primary tabular-nums">{data.total}</span>
            </div>
            <div className="flex flex-col px-4 py-3 bg-surface border border-default rounded-xl min-w-24">
              <span className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted mb-1">Cache TTL</span>
              <TTLBadge ttl={data.cache_ttl_sec} />
            </div>
          </div>

          {data.cache_ttl_sec === -2 ? (
            <EmptyState>
              No trending data — run{' '}
              <CodeBadge>make run-cron</CodeBadge>{' '}
              to populate the trending cache.
            </EmptyState>
          ) : data.items.length === 0 ? (
            <p className="text-sm text-muted">No items in trending window.</p>
          ) : (
            <Panel>
              <Table>
                <Thead>
                  <Th>#</Th>
                  <Th>Object ID</Th>
                  <Th>Score</Th>
                  <Th>Cache TTL</Th>
                </Thead>
                <Tbody>
                  {data.items.map((item, i) => (
                    <Tr key={item.object_id} hoverable>
                      <Td muted mono>{i + 1}</Td>
                      <Td><CodeBadge>{item.object_id}</CodeBadge></Td>
                      <Td mono>{item.score.toFixed(2)}</Td>
                      <Td><TTLBadge ttl={item.cache_ttl_sec} /></Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </Panel>
          )}
        </>
      )}
    </div>
  )
}
