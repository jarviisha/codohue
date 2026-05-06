import { useTrending } from '../hooks/useTrending'
import ErrorBanner from '../components/ErrorBanner'
import { Badge, CodeBadge, EmptyState, LoadingState, MetricTile, PageHeader, PageShell, Panel, Table, Thead, Th, Tbody, Tr, Td } from '../components/ui'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { formatCount, formatTTL } from '../utils/format'

function TTLBadge({ ttl }: { ttl: number }) {
  const missing = ttl === -2
  return (
    <Badge tone={missing ? 'danger' : 'success'} className="tabular-nums">
      {formatTTL(ttl)}
    </Badge>
  )
}

export default function TrendingPage() {
  const { namespace } = useActiveNamespace()
  const { data, error, isLoading } = useTrending(namespace)

  return (
    <PageShell>
      <PageHeader title="Trending Items" />
      {error && <ErrorBanner message="Failed to load trending data." />}
      {isLoading && <LoadingState />}

      {data && (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <MetricTile label="Window" value={`${data.window_hours}h`} />
            <MetricTile label="Total items" value={formatCount(data.total)} />
            <MetricTile label="Cache TTL" value={<TTLBadge ttl={data.cache_ttl_sec} />} />
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
            <Panel bodyClassName="overflow-x-auto">
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
    </PageShell>
  )
}
