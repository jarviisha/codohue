import { useState } from 'react'
import { useParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Button,
  Container,
  EmptyState,
  Inline,
  Pagination,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from '@jarviisha/davinci-react-ui'
import { useTrending } from '@/services/trending'
import PageHeader from '@/components/shell/PageHeader'

const PAGE_SIZE = 50
const WINDOW_OPTIONS = [
  { value: 0, label: 'config default' },
  { value: 1, label: '1h' },
  { value: 6, label: '6h' },
  { value: 24, label: '24h' },
  { value: 168, label: '7d' },
] as const

/**
 * TrendingPage surfaces the Redis-backed trending ZSET for one namespace.
 *
 * Operators use it to sanity-check the cold-start path: when a subject has
 * fewer than 5 interactions the recommend service blends 70% trending + 30%
 * CF, so an empty or stale trending list directly degrades cold rec quality.
 *
 * The header pill renders the namespace-level cache TTL (the ZSET's TTL).
 * `cache_ttl_sec` on individual rows is reserved for per-object TTLs which
 * the backend doesn't yet populate — kept in the wire type for parity.
 */
export default function TrendingPage() {
  const { ns } = useParams<{ ns: string }>()
  const [offset, setOffset] = useState(0)
  const [windowHours, setWindowHours] = useState<number>(0)

  const trending = useTrending(ns ?? null, { limit: PAGE_SIZE, offset, windowHours })

  if (!ns) return null

  const total = trending.data?.total ?? 0
  const items = trending.data?.items ?? []
  const cacheTTL = trending.data?.cache_ttl_sec ?? null

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Inline gap="200" align="center" justify="between" className="w-full" wrap>
          <Stack gap="025">
            <Inline gap="100" align="center">
              <h1 className="text-foreground text-xl font-semibold">Trending · {ns}</h1>
              {cacheTTL != null && <CacheTTLBadge ttl={cacheTTL} />}
            </Inline>
            <p className="text-foreground-subtle text-sm">
              Redis ZSET surfaced for the cold-start recommendation path. Auto-refreshes every 30
              seconds.
            </p>
          </Stack>
          <Inline gap="050" align="center">
            {WINDOW_OPTIONS.map((w) => (
              <Button
                key={w.value}
                size="sm"
                variant={windowHours === w.value ? 'solid' : 'ghost'}
                tone="neutral"
                onClick={() => {
                  setWindowHours(w.value)
                  setOffset(0)
                }}
              >
                {w.label}
              </Button>
            ))}
          </Inline>
        </Inline>
      </PageHeader>

      <Stack gap="300">
        {trending.isError && (
          <Alert
            variant="danger"
            title="Could not load trending data"
            description={trending.error?.message ?? 'unknown error'}
          />
        )}

        {trending.isLoading ? (
          <Skeleton className="h-60 w-full" />
        ) : items.length === 0 ? (
          <EmptyState
            title="Trending ZSET is empty"
            description={
              cacheTTL === -2
                ? 'The Redis key has not been written yet. Wait for the cron Trending phase to run, or trigger a batch run manually.'
                : 'No events landed inside the current window. Try a wider window or wait for ingest to land more activity.'
            }
          />
        ) : (
          <Stack gap="100">
            <Inline gap="100" align="center" justify="between" wrap>
              <span className="text-foreground-subtle text-xs tabular-nums">
                {total.toLocaleString()} entr{total === 1 ? 'y' : 'ies'} · window{' '}
                {trending.data?.window_hours
                  ? `${trending.data.window_hours}h`
                  : 'config default'}{' '}
                · generated {new Date(trending.data!.generated_at).toLocaleTimeString()}
              </span>
            </Inline>
            <TableContainer>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead align="right">Rank</TableHead>
                    <TableHead>Object ID</TableHead>
                    <TableHead align="right">Score</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((it, i) => (
                    <TableRow key={`${it.object_id}-${i}`}>
                      <TableCell align="right" className="tabular-nums">
                        {offset + i + 1}
                      </TableCell>
                      <TableCell>
                        <code className="text-foreground text-xs">{it.object_id}</code>
                      </TableCell>
                      <TableCell align="right" className="tabular-nums">
                        {it.score.toFixed(6)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
            <Pagination
              page={Math.floor(offset / PAGE_SIZE) + 1}
              pageCount={Math.max(1, Math.ceil(total / PAGE_SIZE))}
              onPageChange={(page) => setOffset((page - 1) * PAGE_SIZE)}
            />
          </Stack>
        )}
      </Stack>
    </Container>
  )
}

function CacheTTLBadge({ ttl }: { ttl: number }) {
  if (ttl === -2) return <Badge variant="warning">redis key missing</Badge>
  if (ttl === -1) return <Badge variant="neutral">no TTL</Badge>
  if (ttl < 60) return <Badge variant="warning">{`expires in ${ttl}s`}</Badge>
  return <Badge variant="success">{`fresh · ${Math.round(ttl / 60)}m left`}</Badge>
}
