import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import {
  Token,
  Banner,
  Button,
  EmptyState,
  Pagination,
  Selector,
  Skeleton,
  Stack,
  Table,
  proportional,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from '@astryxdesign/core'
import QueryFeedback from '@/components/QueryFeedback'
import ListSearch from '@/components/ListSearch'
import { readPage } from '@/services/operatorUx'
import PageContainer from '@/components/PageContainer'
import {
  CATALOG_STATE_COLOR,
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

export default function CatalogItemsPage() {
  const { ns } = useParams<{ ns: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const rawState = searchParams.get('state') ?? ''
  const stateFilter = STATE_OPTIONS.some((option) => option.value === rawState) ? rawState : ''
  const search = searchParams.get('q') ?? ''
  const page = readPage(searchParams.get('page'))

  // The author filter lives in the URL rather than local state so the subject
  // inspector can deep-link into "everything this subject authored".
  const author = searchParams.get('author') ?? ''

  const updateFilter = (key: string, value: string) => {
    const params = new URLSearchParams(searchParams)
    if (value) params.set(key, value)
    else params.delete(key)
    if (key !== 'page') params.delete('page')
    setSearchParams(params)
  }

  const items = useCatalogItems(ns ?? null, {
    state: stateFilter || undefined,
    objectId: search || undefined,
    author: author || undefined,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  })

  const applyAuthor = (next: string) => updateFilter('author', next)

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
              {items.data
                ? `${items.data.total} matching. Open an object to see its details.`
                : 'Browse catalog items and embedding status.'}
            </p>
          </Stack>
          <Button
            href={`/ns/${encodeURIComponent(ns)}/catalog`}
            variant="ghost"
            size="sm"
            label="← Status"
          />
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        <Stack gap={4} direction="horizontal" align="center" wrap="wrap">
          <Selector
            size="sm"
            label="State"
            value={stateFilter}
            onChange={(next) => {
              updateFilter('state', next)
            }}
            options={STATE_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
          />
          <ListSearch
            key={`${ns}:${search}`}
            value={search}
            label="Search catalog items"
            description="Object ID contains this text."
            onApply={(value) => updateFilter('q', value)}
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
              <Button size="sm" variant="ghost" onClick={() => applyAuthor('')} label="Clear" />
            </Stack>
          )}
          {items.data && <span className="text-secondary text-sm ml-auto">page {page + 1}</span>}
        </Stack>

        {redrive.error && (
          <Banner status="error" title="Redrive failed" description={redrive.error.message} />
        )}
        {remove.error && (
          <Banner status="error" title="Delete failed" description={remove.error.message} />
        )}

        <QueryFeedback query={items} label="Catalog items" />
        {items.isLoading && <Skeleton className="h-48 w-full" />}

        {items.data && items.data.items.length === 0 && (
          <EmptyState
            title="No items match"
            description="Try clearing the filter or check the Status page for ingest state."
          />
        )}

        {items.data && items.data.items.length > 0 && (
          <Stack className="min-w-0">
            <Table
              aria-label="Catalog items"
              columns={[
                'Object',
                'Author',
                'State',
                'Attempts',
                'Last error',
                'Updated',
                'Actions',
              ].map((key) => ({ key, header: key, width: proportional(1) }))}
            >
              <TableHeader>
                <TableRow>
                  <TableHeaderCell>Object</TableHeaderCell>
                  <TableHeaderCell>Author</TableHeaderCell>
                  <TableHeaderCell>State</TableHeaderCell>
                  <TableHeaderCell className="text-right">Attempts</TableHeaderCell>
                  <TableHeaderCell>Last error</TableHeaderCell>
                  <TableHeaderCell>Updated</TableHeaderCell>
                  <TableHeaderCell className="text-right">Actions</TableHeaderCell>
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
          </Stack>
        )}

        {items.data && page > 0 && items.data.items.length === 0 && (
          <Button label="Return to first page" onClick={() => updateFilter('page', '')} />
        )}
        {items.data && items.data.total > PAGE_SIZE && (
          <Stack align="center" gap={4} direction="horizontal" justify="end" wrap="wrap">
            <Pagination
              page={page + 1}
              totalPages={Math.max(1, Math.ceil(items.data.total / PAGE_SIZE))}
              onChange={(p) => updateFilter('page', String(p))}
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
            onClick={() => onFilterAuthor(item.author_subject_id!)}
            label={item.author_subject_id}
          />
        ) : (
          <span className="text-secondary text-xs">—</span>
        )}
      </TableCell>
      <TableCell>
        <Token color={CATALOG_STATE_COLOR[item.state] ?? 'gray'} label={item.state} />
      </TableCell>
      <TableCell className="text-right tabular-nums">{item.attempt_count}</TableCell>
      <TableCell className="text-secondary text-xs">
        {item.last_error ? (
          <details>
            <summary>View error</summary>
            <p className="whitespace-pre-wrap break-words">{item.last_error}</p>
          </details>
        ) : (
          '—'
        )}
      </TableCell>
      <TableCell className="text-secondary text-sm">
        {new Date(item.updated_at).toLocaleString()}
      </TableCell>
      <TableCell className="text-right">
        <Stack align="center" gap={4} direction="horizontal" justify="end" wrap="wrap">
          {canRedrive && (
            <Button
              size="sm"
              variant="ghost"
              aria-label={`Retry embedding ${item.object_id}`}
              onClick={onRedrive}
              isDisabled={redriving}
              label={redriving ? 'Retrying…' : 'Retry'}
            />
          )}
          <Button
            size="sm"
            variant="ghost"
            aria-label={`Delete ${item.object_id}`}
            onClick={onDelete}
            isDisabled={deleting}
            label={deleting ? 'Deleting…' : 'Delete'}
          />
        </Stack>
      </TableCell>
    </TableRow>
  )
}
