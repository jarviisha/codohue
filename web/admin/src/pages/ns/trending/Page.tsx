import { useParams, useSearchParams } from 'react-router-dom'
import {
  Button,
  EmptyState,
  Field,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  Pagination,
  Panel,
  Select,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Toolbar,
  Tr,
  useRegisterCommand,
} from '@/components/ui'
import {
  TRENDING_WINDOWS,
  useTrending,
  type TrendingWindowHours,
} from '@/services/trending'
import { formatNumber, formatTimestamp } from '@/utils/format'
import { nonNegativeInt, positiveInt } from '@/utils/searchParams'

const DEFAULT_LIMIT = 50
const DEFAULT_WINDOW: TrendingWindowHours = 24

const VALID_WINDOW_VALUES = TRENDING_WINDOWS.map((w) => w.value) as readonly number[]

function parseWindow(raw: string | null): TrendingWindowHours {
  const parsed = Number(raw)
  if (VALID_WINDOW_VALUES.includes(parsed)) return parsed as TrendingWindowHours
  return DEFAULT_WINDOW
}

// Top items in a namespace's Redis trending ZSET. Window selector and limit
// mirror to the URL so views are shareable; per-item and namespace-level
// Redis TTLs are surfaced so operators can see when the cache will refresh.
export default function TrendingPage() {
  const { name = '' } = useParams<{ name: string }>()
  const [searchParams, setSearchParams] = useSearchParams()

  const limit = positiveInt(searchParams.get('limit'), DEFAULT_LIMIT)
  const offset = nonNegativeInt(searchParams.get('offset'), 0)
  const windowHours = parseWindow(searchParams.get('window_hours'))

  const update = (next: Partial<{ window_hours: TrendingWindowHours; limit: number; offset: number }>) => {
    const sp = new URLSearchParams(searchParams)
    if (next.window_hours !== undefined) {
      if (next.window_hours === DEFAULT_WINDOW) sp.delete('window_hours')
      else sp.set('window_hours', String(next.window_hours))
      sp.delete('offset')
    }
    if (next.limit !== undefined) {
      if (next.limit === DEFAULT_LIMIT) sp.delete('limit')
      else sp.set('limit', String(next.limit))
      sp.delete('offset')
    }
    if (next.offset !== undefined) {
      if (next.offset === 0) sp.delete('offset')
      else sp.set('offset', String(next.offset))
    }
    setSearchParams(sp)
  }

  const trending = useTrending({
    namespace: name,
    limit,
    offset,
    window_hours: windowHours,
  })

  useRegisterCommand(
    `ns.${name}.trending.refresh`,
    `Refresh ${name} trending`,
    () => void trending.refetch(),
    name,
  )

  const data = trending.data
  const rows = data?.items ?? []

  return (
    <PageShell>
      <PageHeader title="trending" />

      <Panel
        title={`trending top ${rows.length || limit}`}
        actions={
          <>
            <span className="font-mono text-xs text-muted">
              cache ttl {formatTTL(data?.cache_ttl_sec)}
            </span>
            {data?.generated_at ? (
              <span className="font-mono text-xs text-muted">
                · generated {formatTimestamp(data.generated_at)}
              </span>
            ) : null}
            <Button
              variant="ghost"
              size="sm"
              loading={trending.isFetching && !trending.isLoading}
              onClick={() => void trending.refetch()}
            >
              Refresh
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          {trending.isError ? (
            <Notice tone="fail" title="Failed to load trending">
              {(trending.error as Error)?.message ?? 'Unable to load trending items.'}
            </Notice>
          ) : null}

          <Toolbar>
            <Field label="window" htmlFor="trending-window">
              <Select
                id="trending-window"
                selectSize="sm"
                value={String(windowHours)}
                onChange={(event) =>
                  update({ window_hours: Number(event.target.value) as TrendingWindowHours })
                }
              >
                {TRENDING_WINDOWS.map((option) => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </Select>
            </Field>
            <Field label="limit" htmlFor="trending-limit">
              <Select
                id="trending-limit"
                selectSize="sm"
                value={String(limit)}
                onChange={(event) => update({ limit: Number(event.target.value) })}
              >
                {[25, 50, 100, 200].map((value) => (
                  <option key={value} value={value}>{value}</option>
                ))}
              </Select>
            </Field>
          </Toolbar>

          {trending.isLoading ? (
            <LoadingState rows={8} label="loading trending" />
          ) : rows.length === 0 && !trending.isError ? (
            <EmptyState
              title="No trending items"
              description="Ingest more events or widen the window to populate trending."
            />
          ) : (
            <>
              <Table>
                <Thead>
                  <Tr>
                    <Th align="right">rank</Th>
                    <Th>object_id</Th>
                    <Th align="right">score</Th>
                    <Th align="right">ttl</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {rows.map((item, idx) => (
                    <Tr key={item.object_id}>
                      <Td mono align="right">{formatNumber(offset + idx + 1)}</Td>
                      <Td mono>{item.object_id}</Td>
                      <Td mono align="right">{formatScore(item.score)}</Td>
                      <Td mono align="right">{formatTTL(item.cache_ttl_sec)}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>

              <Pagination
                offset={offset}
                limit={limit}
                total={data?.total}
                onOffsetChange={(next) => update({ offset: next })}
              />
            </>
          )}
        </div>
      </Panel>
    </PageShell>
  )
}

// Trending scores are floats; one decimal keeps the column readable while
// still differentiating items at the tail of the ranking.
function formatScore(n: number): string {
  if (Number.isNaN(n)) return '—'
  return n.toLocaleString('en-US', { minimumFractionDigits: 1, maximumFractionDigits: 1 })
}

// -1 = key has no expiry, -2 = key missing entirely. Both come straight from
// Redis TTL semantics and are surfaced verbatim so operators can correlate
// with `TTL` debugging.
function formatTTL(seconds: number | undefined): string {
  if (seconds === undefined) return '—'
  if (seconds === -1) return 'no expiry'
  if (seconds === -2) return 'missing'
  if (seconds < 60) return `${seconds}s`
  const m = Math.floor(seconds / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  return `${h}h`
}
