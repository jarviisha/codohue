import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Button,
  Card,
  CardContent,
  Container,
  Inline,
  Skeleton,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from '@jarviisha/davinci-react-ui'
import {
  useSubjectProfile,
  useSubjectRecommendations,
  type RecommendDebug,
  type RecommendDebugItem,
} from '@/services/subjects'
import PageHeader from '@/components/shell/PageHeader'
import MetaLine from '@/components/MetaLine'

/**
 * SubjectInspectorPage is the operator's "why did user X get rec Y?" answer.
 *
 * Top half — SubjectProfile: interaction count, sparse vector NNZ (Qdrant
 * presence indicator), and a sample of recently-seen items. Sparse NNZ = -1
 * means the cron hasn't indexed this subject yet (cold path) — surfaced as a
 * warning badge.
 *
 * Bottom half — Recommendations w/ optional debug toggle. The debug block
 * carries the sparse_nnz / dense_score / alpha that drove the score blend,
 * so operators can correlate weak recs against thin signal.
 */
export default function SubjectInspectorPage() {
  const { ns, id } = useParams<{ ns: string; id: string }>()
  const [debug, setDebug] = useState(true)
  const [limit, setLimit] = useState(20)

  const profile = useSubjectProfile(ns ?? null, id ?? null)
  const recs = useSubjectRecommendations(ns ?? null, id ?? null, { limit, debug })

  if (!ns || !id) return null

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Inline align="center" justify="between" className="w-full" wrap>
          <Stack gap="050">
            <h1 className="text-foreground text-xl font-semibold">Subject {id}</h1>
          </Stack>
          <Inline align="center">
            <Switch
              checked={debug}
              onChange={(e) => setDebug(e.target.checked)}
              label="Debug"
            />
            <Link to={`/ns/${encodeURIComponent(ns)}/events?subject_id=${encodeURIComponent(id)}`}>
              <Button variant="outline" tone="neutral" size="sm">
                View events →
              </Button>
            </Link>
            {/* Objects this subject authored — ownership metadata, unrelated to
                the interactions above, which is why it links out rather than
                folding into the profile card. */}
            <Link to={`/ns/${encodeURIComponent(ns)}/catalog/items?author=${encodeURIComponent(id)}`}>
              <Button variant="outline" tone="neutral" size="sm">
                Authored objects →
              </Button>
            </Link>
          </Inline>
        </Inline>
      </PageHeader>

      <Stack>
        {profile.data && (profile.data.sparse_vector_nnz < 0 || profile.data.interaction_count === 0) && (
          <Alert
            variant="warning"
            title={
              profile.data.interaction_count === 0
                ? 'No interactions recorded for this subject'
                : 'Subject not yet indexed in Qdrant'
            }
            description={
              profile.data.interaction_count === 0
                ? 'Recommendations will fall back to the trending path. Inject a test event or wait for ingest to land real activity.'
                : 'The cron job has not run since this subject\'s first event. Recommendations will use the cold-start path until the next batch run completes.'
            }
            actions={
              <Inline>
                <Link
                  to={`/ns/${encodeURIComponent(ns)}/events?subject_id=${encodeURIComponent(id)}`}
                >
                  <Button size="sm" variant="ghost">
                    Open events
                  </Button>
                </Link>
                <Link to={`/ns/${encodeURIComponent(ns)}/batch-runs`}>
                  <Button size="sm" variant="ghost">
                    Batch runs
                  </Button>
                </Link>
              </Inline>
            }
          />
        )}

        <SubjectProfileCard
          loading={profile.isLoading}
          error={profile.error}
          interactionCount={profile.data?.interaction_count}
          sparseNNZ={profile.data?.sparse_vector_nnz}
          seenItems={profile.data?.seen_items ?? []}
          seenItemsDays={profile.data?.seen_items_days}
        />

        <Stack>
          <Inline align="center" justify="between">
            <Stack>
              <h2 className="text-foreground text-sm font-semibold">Recommendations</h2>
              {recs.data ? (
                <MetaLine
                  size="xs"
                  items={[
                    `source=${recs.data.source}`,
                    `${recs.data.total.toLocaleString()} total`,
                    `generated ${new Date(recs.data.generated_at).toLocaleTimeString()}`,
                  ]}
                />
              ) : (
                <p className="text-foreground-subtle text-xs">
                  Live from /v1/subjects/:id/recommendations via the admin proxy.
                </p>
              )}
            </Stack>
            <Inline align="center">
              {[10, 20, 50, 100].map((n) => (
                <Button
                  key={n}
                  size="sm"
                  variant={limit === n ? 'solid' : 'ghost'}
                  tone="neutral"
                  onClick={() => setLimit(n)}
                >
                  {n}
                </Button>
              ))}
            </Inline>
          </Inline>

          {recs.isLoading ? (
            <Skeleton className="h-40 w-full" />
          ) : recs.isError ? (
            <Alert
              variant="danger"
              title="Could not load recommendations"
              description={recs.error?.message ?? 'unknown error'}
            />
          ) : (
            <Stack>
              {debug && recs.data?.debug && <DebugSummary debug={recs.data.debug} />}
              <RecommendationsTable items={recs.data?.items ?? []} />
            </Stack>
          )}
        </Stack>
      </Stack>
    </Container>
  )
}

function SubjectProfileCard({
  loading,
  error,
  interactionCount,
  sparseNNZ,
  seenItems,
  seenItemsDays,
}: {
  loading: boolean
  error: unknown
  interactionCount?: number
  sparseNNZ?: number
  seenItems: string[]
  seenItemsDays?: number
}) {
  if (loading) return <Skeleton className="h-32 w-full" />
  if (error) {
    return (
      <Alert
        variant="danger"
        title="Could not load profile"
        description={
          error instanceof Error ? error.message : 'unknown error'
        }
      />
    )
  }

  const indexed = sparseNNZ != null && sparseNNZ >= 0
  const tiles: Array<{ label: string; value: string; badge?: { variant: 'success' | 'warning' | 'neutral'; label: string } }> = [
    {
      label: 'Interactions',
      value: (interactionCount ?? 0).toLocaleString(),
    },
    {
      label: 'Sparse vector NNZ',
      value: sparseNNZ == null ? '—' : sparseNNZ < 0 ? 'not indexed' : sparseNNZ.toLocaleString(),
      badge: indexed
        ? { variant: 'success', label: 'in Qdrant' }
        : { variant: 'warning', label: 'cold' },
    },
    {
      label: 'Seen items',
      value: `${seenItems.length.toLocaleString()}${seenItemsDays != null ? ` (last ${seenItemsDays}d)` : ''}`,
    },
  ]

  return (
    <Stack>
      <Inline align="start" wrap>
        {tiles.map((t) => (
          <Card key={t.label} className="flex-1 min-w-40">
            <CardContent>
              <Stack>
                <span className="text-foreground-subtle text-xs uppercase tracking-wide">
                  {t.label}
                </span>
                <Inline align="center">
                  <span className="text-foreground text-xl font-semibold tabular-nums">
                    {t.value}
                  </span>
                  {t.badge && <Badge variant={t.badge.variant}>{t.badge.label}</Badge>}
                </Inline>
              </Stack>
            </CardContent>
          </Card>
        ))}
      </Inline>

      {seenItems.length > 0 && (
        <Card>
          <CardContent>
            <Stack>
              <span className="text-foreground-subtle text-xs uppercase tracking-wide">
                Recent seen items
              </span>
              <Inline wrap>
                {seenItems.slice(0, 30).map((oid) => (
                  <code
                    key={oid}
                    className="text-foreground-subtle text-xs bg-surface-sunken px-2 py-1 rounded"
                  >
                    {oid}
                  </code>
                ))}
                {seenItems.length > 30 && (
                  <span className="text-foreground-subtle text-xs">
                    +{seenItems.length - 30} more
                  </span>
                )}
              </Inline>
            </Stack>
          </CardContent>
        </Card>
      )}
    </Stack>
  )
}

function DebugSummary({ debug }: { debug: RecommendDebug }) {
  const tiles = [
    { label: 'Sparse NNZ', value: debug.sparse_nnz.toLocaleString() },
    { label: 'Dense score (max)', value: debug.dense_score.toFixed(4) },
    { label: 'Alpha', value: debug.alpha.toFixed(2) },
    { label: 'Seen items', value: debug.seen_items_count.toLocaleString() },
    { label: 'Interactions', value: debug.interaction_count.toLocaleString() },
  ]
  return (
    <Card>
      <CardContent>
        <Stack>
          <span className="text-foreground-subtle text-xs uppercase tracking-wide">
            Debug score components
          </span>
          <Inline wrap>
            {tiles.map((t) => (
              <Stack key={t.label}>
                <span className="text-foreground-subtle text-xs">{t.label}</span>
                <span className="text-foreground text-sm font-medium tabular-nums">{t.value}</span>
              </Stack>
            ))}
          </Inline>
        </Stack>
      </CardContent>
    </Card>
  )
}

function RecommendationsTable({ items }: { items: RecommendDebugItem[] }) {
  if (items.length === 0) {
    return (
      <p className="text-foreground-subtle text-sm">No recommendations for this subject.</p>
    )
  }
  return (
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
          {items.map((it) => (
            <TableRow key={`${it.rank}-${it.object_id}`}>
              <TableCell align="right" className="tabular-nums">
                {it.rank}
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
  )
}
