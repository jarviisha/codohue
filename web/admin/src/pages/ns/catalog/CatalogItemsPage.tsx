import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Button,
  Container,
  EmptyState,
  Inline,
  Pagination,
  SearchInput,
  Select,
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
import {
  useCatalogItems,
  useDeleteCatalogItem,
  useRedriveCatalogItem,
  type CatalogItemState,
  type CatalogItemSummary,
} from '@/services/catalog'
import PageHeader from '@/components/shell/PageHeader'

const PAGE_SIZE = 25
const STATE_OPTIONS: Array<{ value: CatalogItemState | ''; label: string }> = [
  { value: '', label: 'all states' },
  { value: 'pending', label: 'pending' },
  { value: 'in_flight', label: 'in flight' },
  { value: 'embedded', label: 'embedded' },
  { value: 'failed', label: 'failed' },
  { value: 'dead_letter', label: 'dead-letter' },
]

const STATE_VARIANT: Record<string, 'neutral' | 'success' | 'warning' | 'danger' | 'primary'> = {
  pending: 'neutral',
  in_flight: 'primary',
  embedded: 'success',
  failed: 'warning',
  dead_letter: 'danger',
}

export default function CatalogItemsPage() {
  const { ns } = useParams<{ ns: string }>()
  const [stateFilter, setStateFilter] = useState<CatalogItemState | ''>('')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(0)

  const items = useCatalogItems(ns ?? null, {
    state: stateFilter || undefined,
    objectId: search || undefined,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  })

  const redrive = useRedriveCatalogItem(ns ?? null)
  const remove = useDeleteCatalogItem(ns ?? null)

  if (!ns) return null

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Inline align="center" justify="between" className="w-full">
          <Stack gap="050">
            <h1 className="text-foreground text-xl font-semibold">Catalog items</h1>
            <p className="text-foreground-subtle text-sm">
              {items.data?.total ?? 0} matching. Click a row to open detail.
            </p>
          </Stack>
          <Link to={`/ns/${encodeURIComponent(ns)}/catalog`}>
            <Button variant="ghost" tone="neutral" size="sm">
              ← Status
            </Button>
          </Link>
        </Inline>
      </PageHeader>

      <Stack>
        <Inline align="center" wrap>
          <Select
            size="sm"
            value={stateFilter}
            onChange={(e) => {
              setStateFilter(e.target.value as CatalogItemState | '')
              setPage(0)
            }}
          >
            {STATE_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </Select>
          <SearchInput
            size="sm"
            placeholder="object_id contains…"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value)
              setPage(0)
            }}
            onClear={() => {
              setSearch('')
              setPage(0)
            }}
          />
          {items.data && (
            <span className="text-foreground-subtle text-sm ml-auto">page {page + 1}</span>
          )}
        </Inline>

        {redrive.error && (
          <Alert variant="danger" title="Redrive failed" description={redrive.error.message} />
        )}
        {remove.error && (
          <Alert variant="danger" title="Delete failed" description={remove.error.message} />
        )}

        {items.isLoading && <Skeleton className="h-48 w-full" />}

        {items.isError && (
          <Alert variant="danger" title="Failed to load items" description={items.error?.message ?? ''} />
        )}

        {items.isSuccess && items.data.items.length === 0 && (
          <EmptyState
            title="No items match"
            description="Try clearing the filter or check the Status page for ingest state."
          />
        )}

        {items.isSuccess && items.data.items.length > 0 && (
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Object</TableHead>
                  <TableHead>State</TableHead>
                  <TableHead align="right">Attempts</TableHead>
                  <TableHead>Last error</TableHead>
                  <TableHead>Updated</TableHead>
                  <TableHead align="right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.data.items.map((it) => (
                  <ItemRow
                    key={it.id}
                    ns={ns}
                    item={it}
                    onRedrive={() => redrive.mutate(it.id)}
                    onDelete={() => remove.mutate(it.id)}
                    redriving={redrive.isPending && redrive.variables === it.id}
                    deleting={remove.isPending && remove.variables === it.id}
                  />
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}

        {items.data && items.data.total > PAGE_SIZE && (
          <Inline justify="end">
            <Pagination
              page={page + 1}
              pageCount={Math.max(1, Math.ceil(items.data.total / PAGE_SIZE))}
              onPageChange={(p) => setPage(p - 1)}
            />
          </Inline>
        )}
      </Stack>
    </Container>
  )
}

function ItemRow({
  ns,
  item,
  onRedrive,
  onDelete,
  redriving,
  deleting,
}: {
  ns: string
  item: CatalogItemSummary
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
          className="text-foreground font-medium"
        >
          {item.object_id}
        </Link>
      </TableCell>
      <TableCell>
        <Badge variant={STATE_VARIANT[item.state] ?? 'neutral'}>{item.state}</Badge>
      </TableCell>
      <TableCell align="right" className="tabular-nums">
        {item.attempt_count}
      </TableCell>
      <TableCell className="text-foreground-subtle text-xs">
        {item.last_error ? (
          <span title={item.last_error}>{truncate(item.last_error, 60)}</span>
        ) : (
          '—'
        )}
      </TableCell>
      <TableCell className="text-foreground-subtle text-sm">
        {new Date(item.updated_at).toLocaleString()}
      </TableCell>
      <TableCell align="right">
        <Inline justify="end">
          {canRedrive && (
            <Button size="sm" variant="ghost" onClick={onRedrive} disabled={redriving}>
              {redriving ? 'Redriving…' : 'Redrive'}
            </Button>
          )}
          <Button size="sm" variant="ghost" tone="danger" onClick={onDelete} disabled={deleting}>
            {deleting ? 'Deleting…' : 'Delete'}
          </Button>
        </Inline>
      </TableCell>
    </TableRow>
  )
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s
  return s.slice(0, n - 1) + '…'
}
