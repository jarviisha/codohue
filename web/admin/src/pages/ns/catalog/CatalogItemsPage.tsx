import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import {
  Badge,
  Banner,
  Button,
  EmptyState,
  Pagination,
  Selector,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  TextInput,
} from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import {
  useCatalogItems,
  useDeleteCatalogItem,
  useRedriveCatalogItem,
  type CatalogItemState,
  type CatalogItemSummary,
} from '@/services/catalog'
import PageHeader from '@/components/shell/PageHeader'
import ConfirmDialog from '@/components/ConfirmDialog'

const PAGE_SIZE = 25
const STATE_OPTIONS: Array<{ value: CatalogItemState | ''; label: string }> = [
  { value: '', label: 'all states' },
  { value: 'pending', label: 'pending' },
  { value: 'in_flight', label: 'in flight' },
  { value: 'embedded', label: 'embedded' },
  { value: 'failed', label: 'failed' },
  { value: 'dead_letter', label: 'dead-letter' },
]

const STATE_VARIANT: Record<string, 'neutral' | 'success' | 'warning' | 'error' | 'info'> = {
  pending: 'neutral',
  in_flight: 'info',
  embedded: 'success',
  failed: 'warning',
  dead_letter: 'error',
}

export default function CatalogItemsPage() {
  const { ns } = useParams<{ ns: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const [stateFilter, setStateFilter] = useState<CatalogItemState | ''>('')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(0)

  // The author filter lives in the URL rather than local state so the subject
  // inspector can deep-link into "everything this subject authored".
  const author = searchParams.get('author') ?? ''

  const items = useCatalogItems(ns ?? null, {
    state: stateFilter || undefined,
    objectId: search || undefined,
    author: author || undefined,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  })

  const applyAuthor = (next: string) => {
    const params = new URLSearchParams(searchParams)
    if (next) params.set('author', next)
    else params.delete('author')
    setSearchParams(params)
    setPage(0)
  }

  const redrive = useRedriveCatalogItem(ns ?? null)
  const remove = useDeleteCatalogItem(ns ?? null)
  // Deleting drops the Qdrant point as well as the row, with no undo, so the
  // row button only stages a candidate — the dialog performs it.
  const [pendingDelete, setPendingDelete] = useState<CatalogItemSummary | null>(null)

  if (!ns) return null

  return (
    <PageContainer size="full">
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full">
          <Stack gap={1}>
            <h1 className="text-primary text-xl font-semibold">Catalog items</h1>
            <p className="text-secondary text-sm">
              {items.data?.total ?? 0} matching. Click a row to open detail.
            </p>
          </Stack>
          <Button
            href={`/ns/${encodeURIComponent(ns)}/catalog`}
            variant="ghost"
            
            size="sm" label="← Status" />
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        <Stack gap={4} direction="horizontal" align="center" wrap="wrap">
          <Selector
            size="sm"
            label="State"
            isLabelHidden
            value={stateFilter}
            onChange={(next) => {
              setStateFilter(next as CatalogItemState | '')
              setPage(0)
            }}
            options={STATE_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
          />
          <TextInput
            label="Search catalog items"
            isLabelHidden
            value={search}
            onChange={(next) => {
              setSearch(next)
              setPage(0)
            }}
            hasClear
            size="sm"
            placeholder="object_id contains…"
          />
          {author && (
            <Stack gap={4} direction="horizontal" align="center">
              <span className="text-secondary text-sm">author</span>
              {/* The chip links out to the subject; filtering by author is the
                  in-table click, so each affordance sits where it's wanted. */}
              <Link
                to={`/ns/${encodeURIComponent(ns)}/subjects/${encodeURIComponent(author)}`}
                className="text-primary text-xs"
              >
                {author}
              </Link>
              <Button size="sm" variant="ghost"  onClick={() => applyAuthor('')} label="Clear" />
            </Stack>
          )}
          {items.data && (
            <span className="text-secondary text-sm ml-auto">page {page + 1}</span>
          )}
        </Stack>

        {redrive.error && (
          <Banner status="error" title="Redrive failed" description={redrive.error.message} />
        )}
        {remove.error && (
          <Banner status="error" title="Delete failed" description={remove.error.message} />
        )}

        {items.isLoading && <Skeleton className="h-48 w-full" />}

        {items.isError && (
          <Banner status="error" title="Failed to load items" description={items.error?.message ?? ''} />
        )}

        {items.isSuccess && items.data.items.length === 0 && (
          <EmptyState
            title="No items match"
            description="Try clearing the filter or check the Status page for ingest state."
          />
        )}

        {items.isSuccess && items.data.items.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHeaderCell>Object</TableHeaderCell>
                  <TableHeaderCell>Author</TableHeaderCell>
                  <TableHeaderCell>State</TableHeaderCell>
                  <TableHeaderCell className="text-right" >Attempts</TableHeaderCell>
                  <TableHeaderCell>Last error</TableHeaderCell>
                  <TableHeaderCell>Updated</TableHeaderCell>
                  <TableHeaderCell className="text-right" >Actions</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.data.items.map((it) => (
                  <ItemRow
                    key={it.id}
                    ns={ns}
                    item={it}
                    onFilterAuthor={applyAuthor}
                    onRedrive={() => redrive.mutate(it.id)}
                    onDelete={() => setPendingDelete(it)}
                    redriving={redrive.isPending && redrive.variables === it.id}
                    deleting={remove.isPending && remove.variables === it.id}
                  />
                ))}
              </TableBody>
            </Table>
        )}

        {items.data && items.data.total > PAGE_SIZE && (
          <Stack align="center" gap={4} direction="horizontal" justify="end">
            <Pagination
              page={page + 1}
              totalPages={Math.max(1, Math.ceil(items.data.total / PAGE_SIZE))}
              onChange={(p) => setPage(p - 1)}
            />
          </Stack>
        )}
      </Stack>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(next) => {
          if (!next) setPendingDelete(null)
        }}
        title="Delete catalog item"
        description={
          pendingDelete
            ? `Removes "${pendingDelete.object_id}" from catalog_items and drops its dense vector from Qdrant. The object's events and metadata are untouched, but re-embedding it requires re-ingesting the content. This cannot be undone.`
            : ''
        }
        confirmLabel="Delete item"
        pending={remove.isPending}
        error={remove.error?.message}
        onConfirm={() => {
          if (!pendingDelete) return
          remove.mutate(pendingDelete.id, {
            onSuccess: () => setPendingDelete(null),
          })
        }}
      />
    </PageContainer>
  )
}

function ItemRow({
  ns,
  item,
  onFilterAuthor,
  onRedrive,
  onDelete,
  redriving,
  deleting,
}: {
  ns: string
  item: CatalogItemSummary
  onFilterAuthor: (author: string) => void
  onRedrive: () => void
  onDelete: () => void
  redriving: boolean
  deleting: boolean
}) {
  const canRedrive = item.state === 'failed' || item.state === 'dead_letter'
  return (
    <TableRow>
      <TableCell>
        <Link
          to={`/ns/${encodeURIComponent(ns)}/catalog/items/${item.id}`}
          className="text-primary font-medium"
        >
          {item.object_id}
        </Link>
      </TableCell>
      <TableCell>
        {item.author_subject_id ? (
          <Button
            size="sm"
            variant="ghost"
            
            onClick={() => onFilterAuthor(item.author_subject_id!)} label={item.author_subject_id} />
        ) : (
          <span className="text-secondary text-xs">—</span>
        )}
      </TableCell>
      <TableCell>
        <Badge variant={STATE_VARIANT[item.state] ?? 'neutral'} label={item.state} />
      </TableCell>
      <TableCell  className="text-right tabular-nums">
        {item.attempt_count}
      </TableCell>
      <TableCell className="text-secondary text-xs">
        {item.last_error ? (
          <span title={item.last_error}>{truncate(item.last_error, 60)}</span>
        ) : (
          '—'
        )}
      </TableCell>
      <TableCell className="text-secondary text-sm">
        {new Date(item.updated_at).toLocaleString()}
      </TableCell>
      <TableCell className="text-right" >
        <Stack align="center" gap={4} direction="horizontal" justify="end">
          {canRedrive && (
            <Button size="sm" variant="ghost" onClick={onRedrive} isDisabled={redriving} label={redriving ? 'Redriving…' : 'Redrive'} />
          )}
          <Button size="sm" variant="ghost"  onClick={onDelete} isDisabled={deleting} label={deleting ? 'Deleting…' : 'Delete'} />
        </Stack>
      </TableCell>
    </TableRow>
  )
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s
  return s.slice(0, n - 1) + '…'
}
