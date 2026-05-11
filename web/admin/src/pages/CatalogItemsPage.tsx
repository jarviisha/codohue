import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import ErrorBanner from '../components/ErrorBanner'
import {
  Badge,
  Button,
  EmptyState,
  FormControl,
  Input,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  Panel,
  Select,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Toolbar,
  Tr,
} from '../components/ui'
import {
  useBulkRedriveDeadletter,
  useDeleteCatalogItem,
  useRedriveCatalogItem,
} from '../hooks/useCatalogActions'
import { useCatalogConfig } from '../hooks/useCatalogConfig'
import { useCatalogItems, useCatalogItemDetail } from '../hooks/useCatalogItems'
import { ApiError } from '../services/api'
import type { CatalogItemState, CatalogItemSummary } from '../types'
import { formatCount, formatDateTimeShort } from '../utils/format'
import CatalogItemDetailModal from './namespace-detail/CatalogItemDetailModal'

const PAGE_SIZE = 50

const STATE_OPTIONS: Array<{ label: string; value: CatalogItemState | 'all' }> = [
  { label: 'All states', value: 'all' },
  { label: 'Pending', value: 'pending' },
  { label: 'In flight', value: 'in_flight' },
  { label: 'Embedded', value: 'embedded' },
  { label: 'Failed', value: 'failed' },
  { label: 'Dead-letter', value: 'dead_letter' },
]

// stateBadgeTone maps a catalog item state to the closest semantic Badge tone.
function stateBadgeTone(state: CatalogItemState): 'neutral' | 'success' | 'warning' | 'danger' | 'accent' {
  switch (state) {
    case 'pending':
      return 'neutral'
    case 'in_flight':
      return 'accent'
    case 'embedded':
      return 'success'
    case 'failed':
      return 'warning'
    case 'dead_letter':
      return 'danger'
  }
}

export default function CatalogItemsPage() {
  const { ns } = useParams<{ ns: string }>()
  const namespace = ns ?? ''
  const [state, setState] = useState<CatalogItemState | 'all'>('all')
  const [offset, setOffset] = useState(0)
  const [objectFilter, setObjectFilter] = useState('')
  const [appliedObject, setAppliedObject] = useState('')
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [actionError, setActionError] = useState('')

  const { data, error, isLoading } = useCatalogItems({
    namespace,
    state,
    limit: PAGE_SIZE,
    offset,
    objectID: appliedObject,
  })
  const { data: catalogConfig, error: configErr } = useCatalogConfig(namespace)
  const { data: detail } = useCatalogItemDetail(namespace, selectedID)
  const redrive = useRedriveCatalogItem(namespace)
  const bulkRedrive = useBulkRedriveDeadletter(namespace)
  const deleteItem = useDeleteCatalogItem(namespace)

  const total = data?.total ?? 0
  const pageEnd = Math.min(offset + PAGE_SIZE, total)
  const totalPages = Math.ceil(total / PAGE_SIZE)
  const showBulkRedrive = state === 'dead_letter'
  const catalogNotWired = configErr instanceof ApiError && configErr.status === 503
  const catalogDisabled = !catalogNotWired && catalogConfig != null && !catalogConfig.catalog.enabled
  const hasFilters = state !== 'all' || appliedObject !== ''

  function applyFilter() {
    setAppliedObject(objectFilter.trim())
    setOffset(0)
  }

  function clearFilter() {
    setObjectFilter('')
    setAppliedObject('')
    setOffset(0)
  }

  async function handleRedrive(id: number) {
    setActionError('')
    try {
      await redrive.mutateAsync(id)
    } catch (err) {
      setActionError(formatActionError('Re-drive failed', err))
    }
  }

  async function handleBulkRedrive() {
    setActionError('')
    if (!confirm(`Re-drive every dead-letter item in ${namespace}?`)) return
    try {
      await bulkRedrive.mutateAsync()
    } catch (err) {
      setActionError(formatActionError('Bulk re-drive failed', err))
    }
  }

  async function handleDelete(item: CatalogItemSummary) {
    setActionError('')
    if (!confirm(`Delete catalog item "${item.object_id}"? This removes the dense vector from Qdrant as well.`)) return
    try {
      await deleteItem.mutateAsync(item.id)
    } catch (err) {
      setActionError(formatActionError('Delete failed', err))
    }
  }

  if (!namespace) {
    return (
      <PageShell>
        <PageHeader title="Catalog Items" />
        <ErrorBanner message="Missing namespace in URL." />
      </PageShell>
    )
  }

  return (
    <PageShell>
      <PageHeader
        title={`Catalog Items: ${namespace}`}
        actions={
          <Link
            to={`/namespaces/${encodeURIComponent(namespace)}`}
            className="text-sm font-medium text-accent hover:underline"
          >
            ← Back to settings
          </Link>
        }
      />

      {actionError && (
        <Notice tone="danger" onDismiss={() => setActionError('')}>
          {actionError}
        </Notice>
      )}

      <Panel>
        <Toolbar>
          <FormControl label="State" htmlFor="catalog-state-filter">
            <Select
              id="catalog-state-filter"
              controlSize="sm"
              value={state}
              onChange={e => {
                setState(e.target.value as CatalogItemState | 'all')
                setOffset(0)
              }}
              options={STATE_OPTIONS}
            />
          </FormControl>

          <FormControl label="Object id contains" htmlFor="catalog-obj-filter">
            <Input
              id="catalog-obj-filter"
              inputSize="sm"
              placeholder="e.g. post_42"
              value={objectFilter}
              onChange={e => setObjectFilter(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter') applyFilter()
              }}
            />
          </FormControl>

          <div className="flex gap-2">
            <Button size="sm" variant="ghost" onClick={applyFilter}>
              Apply
            </Button>
            {(appliedObject || objectFilter) && (
              <Button size="sm" variant="ghost" onClick={clearFilter}>
                Clear
              </Button>
            )}
          </div>

          {showBulkRedrive && (
            <div className="ml-auto">
              <Button
                size="sm"
                variant="primary"
                disabled={bulkRedrive.isPending || total === 0}
                onClick={handleBulkRedrive}
              >
                {bulkRedrive.isPending ? 'Re-driving…' : `Re-drive all dead-letter (${formatCount(total)})`}
              </Button>
            </div>
          )}
        </Toolbar>
      </Panel>

      {error && <ErrorBanner message="Failed to load catalog items." />}

      {isLoading && !data && <LoadingState label="Loading catalog items..." />}

      {data && data.items.length === 0 && catalogNotWired && (
        <EmptyState>
          Catalog auto-embedding is not wired in this deployment.
        </EmptyState>
      )}

      {data && data.items.length === 0 && !catalogNotWired && catalogDisabled && !hasFilters && (
        <EmptyState>
          <p className="m-0 mb-3 text-sm text-secondary">
            Catalog auto-embedding is <strong>disabled</strong> for this namespace.
          </p>
          <p className="m-0 text-xs text-muted">
            Enable it in{' '}
            <Link
              to={`/namespaces/${encodeURIComponent(namespace)}`}
              className="font-medium text-accent hover:underline"
            >
              namespace settings
            </Link>{' '}
            to start ingesting raw catalog items via{' '}
            <code>POST /v1/namespaces/{namespace}/catalog</code>.
          </p>
        </EmptyState>
      )}

      {data && data.items.length === 0 && !catalogNotWired && !catalogDisabled && !hasFilters && (
        <EmptyState>
          <p className="m-0 mb-2 text-sm text-secondary">
            No catalog items have been ingested yet.
          </p>
          <p className="m-0 text-xs text-muted">
            Clients can publish raw content to{' '}
            <code>POST /v1/namespaces/{namespace}/catalog</code> — the embedder worker will
            pick them up and produce dense vectors automatically.
          </p>
        </EmptyState>
      )}

      {data && data.items.length === 0 && !catalogNotWired && hasFilters && (
        <EmptyState>
          No catalog items{state !== 'all' ? ` in state "${state}"` : ''}
          {appliedObject ? ` matching "${appliedObject}"` : ''}.
        </EmptyState>
      )}

      {data && data.items.length > 0 && (
        <Panel bodyClassName="overflow-x-auto">
          <Table>
            <Thead>
              <Th>ID</Th>
              <Th>Object ID</Th>
              <Th>State</Th>
              <Th>Strategy</Th>
              <Th align="right">Attempts</Th>
              <Th>Updated</Th>
              <Th align="right">Actions</Th>
            </Thead>
            <Tbody>
              {data.items.map(item => {
                const canRedrive = item.state === 'failed' || item.state === 'dead_letter'
                return (
                  <Tr key={item.id} hoverable>
                    <Td mono>{item.id}</Td>
                    <Td>
                      <button
                        type="button"
                        className="text-sm font-medium text-accent hover:underline"
                        onClick={() => setSelectedID(item.id)}
                      >
                        {item.object_id}
                      </button>
                    </Td>
                    <Td>
                      <Badge tone={stateBadgeTone(item.state)} dot>
                        {item.state}
                      </Badge>
                    </Td>
                    <Td muted mono>
                      {item.strategy_id
                        ? `${item.strategy_id}/${item.strategy_version ?? ''}`
                        : '—'}
                    </Td>
                    <Td align="right" mono>{item.attempt_count}</Td>
                    <Td muted mono className="whitespace-nowrap">
                      {formatDateTimeShort(item.updated_at)}
                    </Td>
                    <Td align="right">
                      <div className="flex justify-end gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!canRedrive || redrive.isPending}
                          onClick={() => handleRedrive(item.id)}
                          title={canRedrive ? 'Re-drive this item' : 'Only failed / dead-letter items can be re-driven'}
                        >
                          Re-drive
                        </Button>
                        <Button
                          size="sm"
                          variant="danger"
                          disabled={deleteItem.isPending}
                          onClick={() => handleDelete(item)}
                        >
                          Delete
                        </Button>
                      </div>
                    </Td>
                  </Tr>
                )
              })}
            </Tbody>
          </Table>

          {totalPages > 1 && (
            <div className="flex items-center justify-between border-t border-default px-2 pt-3">
              <span className="text-xs text-muted">
                {offset + 1}-{pageEnd} of {formatCount(total)}
              </span>
              <div className="flex gap-1">
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={offset === 0}
                  onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                >
                  Prev
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={pageEnd >= total}
                  onClick={() => setOffset(offset + PAGE_SIZE)}
                >
                  Next
                </Button>
              </div>
            </div>
          )}
        </Panel>
      )}

      <CatalogItemDetailModal
        open={selectedID != null}
        item={detail ?? null}
        onClose={() => setSelectedID(null)}
      />
    </PageShell>
  )
}

function formatActionError(prefix: string, err: unknown): string {
  if (err instanceof ApiError) {
    return `${prefix}: ${err.code} — ${err.message}`
  }
  if (err instanceof Error) {
    return `${prefix}: ${err.message}`
  }
  return prefix
}
