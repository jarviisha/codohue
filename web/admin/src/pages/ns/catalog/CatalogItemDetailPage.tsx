import { Link, useNavigate, useParams } from 'react-router-dom'
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
} from '@jarviisha/davinci-react-ui'
import {
  useCatalogItem,
  useDeleteCatalogItem,
  useRedriveCatalogItem,
} from '@/services/catalog'
import MetaLine from '@/components/MetaLine'
import PageHeader from '@/components/shell/PageHeader'

const STATE_VARIANT: Record<string, 'neutral' | 'success' | 'warning' | 'danger' | 'primary'> = {
  pending: 'neutral',
  in_flight: 'primary',
  embedded: 'success',
  failed: 'warning',
  dead_letter: 'danger',
}

export default function CatalogItemDetailPage() {
  const { ns, id } = useParams<{ ns: string; id: string }>()
  const navigate = useNavigate()
  const numericID = id != null ? Number(id) : null
  const q = useCatalogItem(ns ?? null, numericID)
  const redrive = useRedriveCatalogItem(ns ?? null)
  const remove = useDeleteCatalogItem(ns ?? null)

  if (!ns) return null

  if (q.isLoading) {
    return (
      <Container size="full" className="py-6 px-6">
        <Skeleton className="h-48 w-full" />
      </Container>
    )
  }

  if (q.isError) {
    return (
      <Container size="full" className="py-6 px-6">
        <Alert variant="danger" title="Failed to load item" description={q.error?.message ?? ''} />
      </Container>
    )
  }

  const item = q.data
  if (!item) {
    return (
      <Container size="full" className="py-6 px-6">
        <Alert
          variant="warning"
          title="Item not found"
          description="It may have been deleted or never existed."
        />
      </Container>
    )
  }

  const canRedrive = item.state === 'failed' || item.state === 'dead_letter'

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Inline align="center" justify="between" className="w-full">
          <Stack gap="050">
            <Inline align="center">
              <h1 className="text-foreground text-xl font-semibold">{item.object_id}</h1>
              <Badge variant={STATE_VARIANT[item.state] ?? 'neutral'}>{item.state}</Badge>
              {item.strategy_id && (
                <span className="text-foreground-subtle text-xs">
                  {item.strategy_id}@{item.strategy_version}
                </span>
              )}
            </Inline>
            <MetaLine
              items={[
                `${item.attempt_count} attempt${item.attempt_count === 1 ? '' : 's'}`,
                `updated ${new Date(item.updated_at).toLocaleString()}`,
              ]}
            />
          </Stack>
          <Inline>
            {canRedrive && (
              <Button
                size="sm"
                onClick={() => redrive.mutate(item.id)}
                disabled={redrive.isPending}
              >
                {redrive.isPending ? 'Redriving…' : 'Redrive'}
              </Button>
            )}
            <Button
              size="sm"
              variant="outline"
              tone="danger"
              onClick={() =>
                remove.mutate(item.id, {
                  onSuccess: () => navigate(`/ns/${encodeURIComponent(ns)}/catalog/items`),
                })
              }
              disabled={remove.isPending}
            >
              {remove.isPending ? 'Deleting…' : 'Delete'}
            </Button>
          </Inline>
        </Inline>
      </PageHeader>

      <Stack>
        {redrive.error && (
          <Alert variant="danger" title="Redrive failed" description={redrive.error.message} />
        )}
        {remove.error && (
          <Alert variant="danger" title="Delete failed" description={remove.error.message} />
        )}

        {item.last_error && (
          <Alert
            variant={item.state === 'dead_letter' ? 'danger' : 'warning'}
            title="Last error"
            description={item.last_error}
          />
        )}

        <Stack>
          <h2 className="text-foreground text-sm font-semibold">Content</h2>
          <Card>
            <CardContent>
              <pre className="text-foreground text-sm whitespace-pre-wrap wrap-break-word font-mono leading-5">
                {item.content}
              </pre>
            </CardContent>
          </Card>
        </Stack>

        {item.metadata && Object.keys(item.metadata).length > 0 && (
          <Stack>
            <h2 className="text-foreground text-sm font-semibold">Metadata</h2>
            <Card>
              <CardContent>
                <pre className="text-foreground-subtle text-xs whitespace-pre-wrap font-mono leading-5">
                  {JSON.stringify(item.metadata, null, 2)}
                </pre>
              </CardContent>
            </Card>
          </Stack>
        )}

        {item.vector?.preview && (
          <Stack>
            <h2 className="text-foreground text-sm font-semibold">Embedded vector</h2>
            <Card>
              <CardContent>
                <Stack>
                  <Inline align="center">
                    <Tile label="Collection" value={item.vector.collection} />
                    <Tile label="Numeric id" value={item.vector.numeric_id.toLocaleString()} />
                    <Tile label="Dim" value={item.vector.dim.toString()} />
                    <Tile label="L2 norm" value={l2norm(item.vector.preview).toFixed(4)} />
                  </Inline>
                  <Stack>
                    <span className="text-foreground-subtle text-xs uppercase tracking-wide">
                      Preview (first {item.vector.preview.length} dims)
                    </span>
                    <code className="text-foreground-subtle text-xs font-mono break-all leading-5">
                      [{item.vector.preview.map((v) => v.toFixed(4)).join(', ')}]
                    </code>
                  </Stack>
                </Stack>
              </CardContent>
            </Card>
          </Stack>
        )}

        <Inline justify="start">
          <Link to={`/ns/${encodeURIComponent(ns)}/catalog/items`}>
            <Button variant="ghost" tone="neutral" size="sm">
              ← Back to items
            </Button>
          </Link>
        </Inline>
      </Stack>
    </Container>
  )
}

function Tile({ label, value }: { label: string; value: string }) {
  return (
    <Stack>
      <span className="text-foreground-subtle text-xs uppercase tracking-wide">{label}</span>
      <span className="text-foreground text-sm font-semibold tabular-nums">{value}</span>
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
