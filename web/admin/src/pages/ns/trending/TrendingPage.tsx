import { useState } from 'react'
import { useParams } from 'react-router-dom'
import {
  Badge,
  Banner,
  Button,
  EmptyState,
  Pagination,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import { useTrending } from '@/services/trending'
import PageHeader from '@/components/shell/PageHeader'
import MetaLine from '@/components/MetaLine'

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
    <PageContainer size="full">
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full" wrap="wrap">
          <Stack gap={1}>
            <Stack gap={4} direction="horizontal" align="center">
              <h1 className="text-primary text-xl font-semibold">Trending</h1>
              {cacheTTL != null && <CacheTTLBadge ttl={cacheTTL} />}
            </Stack>
            <p className="text-secondary text-sm">
              Redis ZSET surfaced for the cold-start recommendation path. Auto-refreshes every 30
              seconds.
            </p>
          </Stack>
          <Stack gap={4} direction="horizontal" align="center">
            {WINDOW_OPTIONS.map((w) => (
              <Button
                key={w.value}
                size="sm"
                variant="primary"
                
                onClick={() => {
                  setWindowHours(w.value)
                  setOffset(0)
                }} label={w.label} />
            ))}
          </Stack>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        {trending.isError && (
          <Banner
            status="error"
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
          <Stack gap={6}>
            <Stack gap={4} direction="horizontal" align="center" justify="between" wrap="wrap">
              <MetaLine
                size="xs"
                className="tabular-nums"
                items={[
                  `${total.toLocaleString()} entr${total === 1 ? 'y' : 'ies'}`,
                  `window ${
                    trending.data?.window_hours
                      ? `${trending.data.window_hours}h`
                      : 'config default'
                  }`,
                  `generated ${new Date(trending.data!.generated_at).toLocaleTimeString()}`,
                ]}
              />
            </Stack>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHeaderCell className="text-right" >Rank</TableHeaderCell>
                    <TableHeaderCell>Object ID</TableHeaderCell>
                    <TableHeaderCell className="text-right" >Score</TableHeaderCell>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((it, i) => (
                    <TableRow key={`${it.object_id}-${i}`}>
                      <TableCell  className="text-right tabular-nums">
                        {offset + i + 1}
                      </TableCell>
                      <TableCell>
                        <code className="text-primary text-xs">{it.object_id}</code>
                      </TableCell>
                      <TableCell  className="text-right tabular-nums">
                        {it.score.toFixed(6)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            <Pagination
              page={Math.floor(offset / PAGE_SIZE) + 1}
              totalPages={Math.max(1, Math.ceil(total / PAGE_SIZE))}
              onChange={(page) => setOffset((page - 1) * PAGE_SIZE)}
            />
          </Stack>
        )}
      </Stack>
    </PageContainer>
  )
}

function CacheTTLBadge({ ttl }: { ttl: number }) {
  if (ttl === -2) return <Badge variant="warning" label="redis key missing" />
  if (ttl === -1) return <Badge variant="neutral" label="no TTL" />
  if (ttl < 60) return <Badge variant="warning" label={`expires in ${ttl}s`} />
  return <Badge variant="success" label={`fresh ${Math.round(ttl / 60)}m left`} />
}
