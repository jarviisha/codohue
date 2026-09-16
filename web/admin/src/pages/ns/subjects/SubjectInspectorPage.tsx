import { useState } from 'react'
import { useParams } from 'react-router-dom'
import {
  Badge,
  Banner,
  Button,
  Card,
  Skeleton,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
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
    <PageContainer size="full">
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full" wrap="wrap">
          <Stack gap={1}>
            <h1 className="text-primary text-xl font-semibold">Subject {id}</h1>
          </Stack>
          <Stack gap={4} direction="horizontal" align="center">
            <Switch
              value={debug}
              onChange={setDebug}
              label="Debug"
            />
            <Button
              href={`/ns/${encodeURIComponent(ns)}/events?subject_id=${encodeURIComponent(id)}`}
              variant="secondary"
              
              size="sm" label="View events →" />
            {/* Objects this subject authored — ownership metadata, unrelated to
                the interactions above, which is why it links out rather than
                folding into the profile card. */}
            <Button
              href={`/ns/${encodeURIComponent(ns)}/catalog/items?author=${encodeURIComponent(id)}`}
              variant="secondary"
              
              size="sm" label="Authored objects →" />
          </Stack>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        {profile.data && (profile.data.sparse_vector_nnz < 0 || profile.data.interaction_count === 0) && (
          <Banner
            status="warning"
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
            endContent={
              <Stack align="center" gap={4} direction="horizontal">
                <Button
                  href={`/ns/${encodeURIComponent(ns)}/events?subject_id=${encodeURIComponent(id)}`}
                  size="sm"
                  variant="ghost" label="Open events" />
                <Button href={`/ns/${encodeURIComponent(ns)}/batch-runs`} size="sm" variant="ghost" label="Batch runs" />
              </Stack>
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

        <Stack gap={6}>
          <Stack gap={4} direction="horizontal" align="center" justify="between">
            <Stack gap={6}>
              <h2 className="text-primary text-sm font-semibold">Recommendations</h2>
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
                <p className="text-secondary text-xs">
                  Live from /v1/subjects/:id/recommendations via the admin proxy.
                </p>
              )}
            </Stack>
            <Stack gap={4} direction="horizontal" align="center">
              {[10, 20, 50, 100].map((n) => (
                <Button
                  key={n}
                  size="sm"
                  variant="primary"
                  
                  onClick={() => setLimit(n)} label={String(n)} />
              ))}
            </Stack>
          </Stack>

          {recs.isLoading ? (
            <Skeleton className="h-40 w-full" />
          ) : recs.isError ? (
            <Banner
              status="error"
              title="Could not load recommendations"
              description={recs.error?.message ?? 'unknown error'}
            />
          ) : (
            <Stack gap={6}>
              {debug && recs.data?.debug && <DebugSummary debug={recs.data.debug} />}
              <RecommendationsTable items={recs.data?.items ?? []} />
            </Stack>
          )}
        </Stack>
      </Stack>
    </PageContainer>
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
      <Banner
        status="error"
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
    <Stack gap={6}>
      <Stack gap={4} direction="horizontal" align="start" wrap="wrap">
        {tiles.map((t) => (
          <Card key={t.label} className="flex-1 min-w-40">
              <Stack gap={6}>
                <span className="text-secondary text-xs uppercase tracking-wide">
                  {t.label}
                </span>
                <Stack gap={4} direction="horizontal" align="center">
                  <span className="text-primary text-xl font-semibold tabular-nums">
                    {t.value}
                  </span>
                  {t.badge && <Badge variant={t.badge.variant} label={t.badge.label} />}
                </Stack>
              </Stack>
          </Card>
        ))}
      </Stack>

      {seenItems.length > 0 && (
        <Card>
            <Stack gap={6}>
              <span className="text-secondary text-xs uppercase tracking-wide">
                Recent seen items
              </span>
              <Stack align="center" gap={4} direction="horizontal" wrap="wrap">
                {seenItems.slice(0, 30).map((oid) => (
                  <code
                    key={oid}
                    className="text-secondary text-xs bg-muted px-2 py-1 rounded"
                  >
                    {oid}
                  </code>
                ))}
                {seenItems.length > 30 && (
                  <span className="text-secondary text-xs">
                    +{seenItems.length - 30} more
                  </span>
                )}
              </Stack>
            </Stack>
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
        <Stack gap={6}>
          <span className="text-secondary text-xs uppercase tracking-wide">
            Debug score components
          </span>
          <Stack align="center" gap={4} direction="horizontal" wrap="wrap">
            {tiles.map((t) => (
              <Stack gap={6} key={t.label}>
                <span className="text-secondary text-xs">{t.label}</span>
                <span className="text-primary text-sm font-medium tabular-nums">{t.value}</span>
              </Stack>
            ))}
          </Stack>
        </Stack>
    </Card>
  )
}

function RecommendationsTable({ items }: { items: RecommendDebugItem[] }) {
  if (items.length === 0) {
    return (
      <p className="text-secondary text-sm">No recommendations for this subject.</p>
    )
  }
  return (
      <Table>
        <TableHeader>
          <TableRow>
            <TableHeaderCell className="text-right" >Rank</TableHeaderCell>
            <TableHeaderCell>Object ID</TableHeaderCell>
            <TableHeaderCell className="text-right" >Score</TableHeaderCell>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((it) => (
            <TableRow key={`${it.rank}-${it.object_id}`}>
              <TableCell  className="text-right tabular-nums">
                {it.rank}
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
  )
}
