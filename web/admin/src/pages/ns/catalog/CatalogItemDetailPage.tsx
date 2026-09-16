import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Badge, Banner, Button, Card, Skeleton, Stack } from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import {
  useCatalogItem,
  useDeleteCatalogItem,
  useRedriveCatalogItem,
} from '@/services/catalog'
import MetaLine from '@/components/MetaLine'
import PageHeader from '@/components/shell/PageHeader'
import ConfirmDialog from '@/components/ConfirmDialog'

const STATE_VARIANT: Record<string, 'neutral' | 'success' | 'warning' | 'error' | 'info'> = {
  pending: 'neutral',
  in_flight: 'info',
  embedded: 'success',
  failed: 'warning',
  dead_letter: 'error',
}

export default function CatalogItemDetailPage() {
  const { ns, id } = useParams<{ ns: string; id: string }>()
  const navigate = useNavigate()
  const numericID = id != null ? Number(id) : null
  const q = useCatalogItem(ns ?? null, numericID)
  const redrive = useRedriveCatalogItem(ns ?? null)
  const remove = useDeleteCatalogItem(ns ?? null)
  const [confirmOpen, setConfirmOpen] = useState(false)

  if (!ns) return null

  if (q.isLoading) {
    return (
      <PageContainer size="full">
        <Skeleton className="h-48 w-full" />
      </PageContainer>
    )
  }

  if (q.isError) {
    return (
      <PageContainer size="full">
        <Banner status="error" title="Failed to load item" description={q.error?.message ?? ''} />
      </PageContainer>
    )
  }

  const item = q.data
  if (!item) {
    return (
      <PageContainer size="full">
        <Banner
          status="warning"
          title="Item not found"
          description="It may have been deleted or never existed."
        />
      </PageContainer>
    )
  }

  const canRedrive = item.state === 'failed' || item.state === 'dead_letter'

  return (
    <PageContainer size="full">
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full">
          <Stack gap={1}>
            <Stack gap={4} direction="horizontal" align="center">
              <h1 className="text-primary text-xl font-semibold">{item.object_id}</h1>
              <Badge variant={STATE_VARIANT[item.state] ?? 'neutral'} label={item.state} />
              {item.strategy_id && (
                <span className="text-secondary text-xs">
                  {item.strategy_id}@{item.strategy_version}
                </span>
              )}
            </Stack>
            <MetaLine
              items={[
                `${item.attempt_count} attempt${item.attempt_count === 1 ? '' : 's'}`,
                `updated ${new Date(item.updated_at).toLocaleString()}`,
              ]}
            />
            {item.author_subject_id && (
              <Stack gap={4} direction="horizontal" align="center">
                <span className="text-secondary text-xs">authored by</span>
                <Link
                  to={`/ns/${encodeURIComponent(ns)}/subjects/${encodeURIComponent(item.author_subject_id)}`}
                  className="text-primary text-xs"
                >
                  {item.author_subject_id}
                </Link>
              </Stack>
            )}
          </Stack>
          <Stack align="center" gap={4} direction="horizontal">
            {canRedrive && (
              <Button
                size="sm"
                onClick={() => redrive.mutate(item.id)}
                isDisabled={redrive.isPending} label={redrive.isPending ? 'Redriving…' : 'Redrive'} />
            )}
            <Button
              size="sm"
              variant="destructive"
              
              onClick={() => setConfirmOpen(true)}
              isDisabled={remove.isPending} label={remove.isPending ? 'Deleting…' : 'Delete'} />
          </Stack>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        {redrive.error && (
          <Banner status="error" title="Redrive failed" description={redrive.error.message} />
        )}
        {remove.error && (
          <Banner status="error" title="Delete failed" description={remove.error.message} />
        )}

        {item.last_error && (
          <Banner
            status={item.state === 'dead_letter' ? 'error' : 'warning'}
            title="Last error"
            description={item.last_error}
          />
        )}

        <Stack gap={6}>
          <h2 className="text-primary text-sm font-semibold">Content</h2>
          <Card>
              <pre className="text-primary text-sm whitespace-pre-wrap wrap-break-word font-mono leading-5">
                {item.content}
              </pre>
          </Card>
        </Stack>

        {item.metadata && Object.keys(item.metadata).length > 0 && (
          <Stack gap={6}>
            <h2 className="text-primary text-sm font-semibold">Metadata</h2>
            <Card>
                <pre className="text-secondary text-xs whitespace-pre-wrap font-mono leading-5">
                  {JSON.stringify(item.metadata, null, 2)}
                </pre>
            </Card>
          </Stack>
        )}

        {item.vector?.preview && (
          <Stack gap={6}>
            <h2 className="text-primary text-sm font-semibold">Embedded vector</h2>
            <Card>
                <Stack gap={6}>
                  <Stack gap={4} direction="horizontal" align="center">
                    <Tile label="Collection" value={item.vector.collection} />
                    <Tile label="Numeric id" value={item.vector.numeric_id.toLocaleString()} />
                    <Tile label="Dim" value={item.vector.dim.toString()} />
                    <Tile label="L2 norm" value={l2norm(item.vector.preview).toFixed(4)} />
                  </Stack>
                  <Stack gap={6}>
                    <span className="text-secondary text-xs uppercase tracking-wide">
                      Preview (first {item.vector.preview.length} dims)
                    </span>
                    <code className="text-secondary text-xs font-mono break-all leading-5">
                      [{item.vector.preview.map((v) => v.toFixed(4)).join(', ')}]
                    </code>
                  </Stack>
                </Stack>
            </Card>
          </Stack>
        )}

        <Stack align="center" gap={4} direction="horizontal" justify="start">
          <Button
            href={`/ns/${encodeURIComponent(ns)}/catalog/items`}
            variant="ghost"
            
            size="sm" label="← Back to items" />
        </Stack>
      </Stack>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Delete catalog item"
        description={`Removes "${item.object_id}" from catalog_items and drops its dense vector from Qdrant. The object's events and metadata are untouched, but re-embedding it requires re-ingesting the content. This cannot be undone.`}
        confirmLabel="Delete item"
        pending={remove.isPending}
        error={remove.error?.message}
        onConfirm={() =>
          remove.mutate(item.id, {
            onSuccess: () => {
              setConfirmOpen(false)
              navigate(`/ns/${encodeURIComponent(ns)}/catalog/items`)
            },
          })
        }
      />
    </PageContainer>
  )
}

function Tile({ label, value }: { label: string; value: string }) {
  return (
    <Stack gap={6}>
      <span className="text-secondary text-xs uppercase tracking-wide">{label}</span>
      <span className="text-primary text-sm font-semibold tabular-nums">{value}</span>
    </Stack>
  )
}

// l2norm reports the L2 norm of a vector preview — operators eyeball this to
// sanity-check the embedder didn't produce a degenerate (near-zero) vector.
// Only computed over the preview slice, so for an N-dim vector with K dims
// previewed the result is a lower bound on the true norm.
function l2norm(v: number[]): number {
  let s = 0
  for (const x of v) s += x * x
  return Math.sqrt(s)
}
