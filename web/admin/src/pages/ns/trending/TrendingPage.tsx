import { Link, useParams, useSearchParams } from 'react-router-dom'
import {
  Button,
  EmptyState,
  Skeleton,
  Stack,
  Table,
  proportional,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  Token,
} from '@astryxdesign/core'
import QueryFeedback from '@/components/QueryFeedback'
import { readPage } from '@/services/operatorUx'
import { useTrending } from '@/services/trending'
import PageHeader from '@/components/shell/PageHeader'

const PAGE_SIZE = 50

/** Displays the cached ranking; refresh only reads it, while batch runs rebuild it. */
export default function TrendingPage() {
  const { ns } = useParams<{ ns: string }>()
  const [params, setParams] = useSearchParams()
  const page = readPage(params.get('page'))
  const offset = page * PAGE_SIZE
  const trending = useTrending(ns ?? null, { limit: PAGE_SIZE, offset })

  if (!ns) return null

  const data = trending.data
  const prefix = `/ns/${encodeURIComponent(ns)}`
  const setPage = (next: number) => {
    const updated = new URLSearchParams(params)
    if (next <= 0) updated.delete('page')
    else updated.set('page', String(next + 1))
    setParams(updated)
  }

  return (
    <>
      <PageHeader>
        <Stack gap={2}>
          <h1 className="text-primary text-xl font-semibold">Trending</h1>
          <p className="text-secondary text-sm">
            Ranked items from the latest available batch result. This page checks for updates every
            30 seconds; Refresh reloads the result without recalculating scores.
          </p>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        <Stack gap={2}>
          {data && (
            <Stack direction="horizontal" gap={3} align="center" wrap="wrap">
              <Token color="gray" label={`Configured event window: ${data.window_hours} hours`} />
              {data.items.length > 0 && data.cache_ttl_sec > 0 && (
                <Token color="gray" label={`Cache expires in ${data.cache_ttl_sec}s`} />
              )}
              {data.items.length > 0 && data.cache_ttl_sec === -1 && (
                <Token color="gray" label="Cache has no expiry" />
              )}
            </Stack>
          )}
          <p className="text-secondary text-sm">
            The event window is set in Configuration. After changing it, run a batch or wait for the
            next scheduled batch to rebuild the ranking. An unexpired cache does not guarantee
            recent activity.
          </p>
          <Stack direction="horizontal" gap={4} wrap="wrap">
            <Link className="text-primary underline" to={`${prefix}/config`}>
              Configuration
            </Link>
            <Link className="text-primary underline" to={`${prefix}/batch-runs`}>
              Batch runs
            </Link>
            <Link className="text-primary underline" to={`${prefix}/events`}>
              Events
            </Link>
          </Stack>
        </Stack>

        <QueryFeedback query={trending} label="Trending" />

        {trending.isLoading ? (
          <Skeleton height={240} />
        ) : data ? (
          <Stack gap={4}>
            {data.items.length === 0 ? (
              <EmptyState
                title={page > 0 ? 'No items on this page' : 'No trending items available'}
                description={
                  page > 0
                    ? 'Return to the previous page. The ranking may have changed since you last loaded it.'
                    : `The ranking may be empty because there are no eligible events in the configured ${data.window_hours}-hour window, no batch has produced a result yet, or the cached result has expired. Check Events and Batch runs to confirm the cause.`
                }
              />
            ) : (
              <>
                <p className="text-secondary text-sm" role="status">
                  Showing ranks {offset + 1}–{offset + data.items.length}
                </p>
                <Table
                  columns={['Rank', 'Object ID', 'Score'].map((key) => ({
                    key,
                    label: key,
                    width: proportional(1),
                  }))}
                >
                  <TableHeader>
                    <TableRow>
                      <TableHeaderCell className="text-right">Rank</TableHeaderCell>
                      <TableHeaderCell>Object ID</TableHeaderCell>
                      <TableHeaderCell className="text-right">Score</TableHeaderCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {data.items.map((item, i) => (
                      <TableRow key={item.object_id}>
                        <TableCell className="text-right tabular-nums">{offset + i + 1}</TableCell>
                        <TableCell>
                          <code className="text-primary text-xs break-all">{item.object_id}</code>
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {item.score.toFixed(6)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </>
            )}
            {(page > 0 || data.items.length === PAGE_SIZE) && (
              <Stack direction="horizontal" gap={3} align="center">
                <Button
                  label="Previous page"
                  variant="secondary"
                  isDisabled={page === 0}
                  onClick={() => setPage(page - 1)}
                />
                <p className="text-secondary text-sm">Page {page + 1}</p>
                <Button
                  label="Next page"
                  variant="secondary"
                  isDisabled={data.items.length < PAGE_SIZE}
                  onClick={() => setPage(page + 1)}
                />
              </Stack>
            )}
          </Stack>
        ) : null}
      </Stack>
    </>
  )
}
