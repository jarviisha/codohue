import { useState } from 'react'
import { Link, Outlet, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  Pagination,
  Select,
  StatusToken,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Toolbar,
  Tr,
  useRegisterCommand,
} from '../../../../components/ui'
import type {
  CatalogItemState,
  CatalogItemSummary,
  CatalogItemsStateFilter,
} from '../../../../services/catalog'
import {
  useBulkRedriveDeadletter,
  useCatalogConfig,
  useCatalogItems,
  useRedriveCatalogItem,
} from '../../../../services/catalog'
import { paths } from '../../../../routes/path'
import { formatNumber, formatRelative } from '../../../../utils/format'

const STATES: CatalogItemsStateFilter[] = [
  'all',
  'pending',
  'in_flight',
  'embedded',
  'failed',
  'dead_letter',
]

function stateToken(state: CatalogItemState) {
  switch (state) {
    case 'embedded':
      return 'ok'
    case 'in_flight':
      return 'run'
    case 'failed':
    case 'dead_letter':
      return 'fail'
    case 'pending':
      return 'pend'
    default:
      return 'idle'
  }
}

function canRedrive(state: CatalogItemState) {
  return state === 'failed' || state === 'dead_letter'
}

function positiveInt(value: string | null, fallback: number) {
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed < 1) return fallback
  return parsed
}

function nonNegativeInt(value: string | null, fallback: number) {
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed < 0) return fallback
  return parsed
}

export default function CatalogItemsListPage() {
  const { name = '' } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams, setSearchParams] = useSearchParams()

  const state = (searchParams.get('state') || 'all') as CatalogItemsStateFilter
  const objectID = searchParams.get('object_id') ?? ''
  const limit = positiveInt(searchParams.get('limit'), 50)
  const offset = nonNegativeInt(searchParams.get('offset'), 0)

  const items = useCatalogItems({
    namespace: name,
    state,
    object_id: objectID,
    limit,
    offset,
  })
  const catalog = useCatalogConfig(name)
  const redrive = useRedriveCatalogItem()
  const bulkRedrive = useBulkRedriveDeadletter()
  const [redriveTarget, setRedriveTarget] = useState<CatalogItemSummary | null>(null)
  const [showBulkConfirm, setShowBulkConfirm] = useState(false)

  useRegisterCommand(
    `ns.${name}.catalog.config`,
    `Open ${name} catalog config`,
    () => navigate(paths.nsCatalog(name)),
    name,
  )
  useRegisterCommand(
    `ns.${name}.catalog.items.refresh`,
    `Refresh ${name} catalog items`,
    () => {
      void items.refetch()
      void catalog.refetch()
    },
    name,
  )
  useRegisterCommand(
    `ns.${name}.catalog.items.redriveDeadletter`,
    `Redrive ${name} dead-letter catalog items`,
    () => setShowBulkConfirm(true),
    name,
  )

  const setFilter = (next: {
    state?: CatalogItemsStateFilter
    object_id?: string
    offset?: number
    limit?: number
  }) => {
    const sp = new URLSearchParams(searchParams)
    if (next.state !== undefined) {
      if (next.state === 'all') sp.delete('state')
      else sp.set('state', next.state)
      sp.delete('offset')
    }
    if (next.object_id !== undefined) {
      if (next.object_id) sp.set('object_id', next.object_id)
      else sp.delete('object_id')
      sp.delete('offset')
    }
    if (next.limit !== undefined) {
      if (next.limit === 50) sp.delete('limit')
      else sp.set('limit', String(next.limit))
      sp.delete('offset')
    }
    if (next.offset !== undefined) {
      if (next.offset === 0) sp.delete('offset')
      else sp.set('offset', String(next.offset))
    }
    setSearchParams(sp)
  }

  const rows = items.data?.items ?? []
  const deadLetterCount = catalog.data?.backlog.dead_letter ?? 0

  return (
    <PageShell>
      <PageHeader
        title="Catalog items"
        meta={`namespace ${name}`}
        actions={
          <>
            <Button
              variant="ghost"
              size="sm"
              loading={items.isFetching && !items.isLoading}
              onClick={() => {
                void items.refetch()
                void catalog.refetch()
              }}
            >
              Refresh
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => navigate(paths.nsCatalog(name))}
            >
              Config
            </Button>
            <Button
              variant="primary"
              size="sm"
              disabled={deadLetterCount === 0}
              onClick={() => setShowBulkConfirm(true)}
            >
              Redrive deadletter ({formatNumber(deadLetterCount)})
            </Button>
          </>
        }
      />

      {items.isError ? (
        <Notice tone="fail" title="Failed to load catalog items">
          {(items.error as Error)?.message ?? 'Unable to load catalog items.'}
        </Notice>
      ) : null}

      {redrive.isSuccess ? (
        <Notice tone="ok" title="Catalog item queued" onDismiss={() => redrive.reset()}>
          Item {redrive.data.object_id} was reset to {redrive.data.state}.
        </Notice>
      ) : redrive.isError ? (
        <Notice tone="fail" title="Redrive failed">
          {(redrive.error as Error)?.message ?? 'Unable to redrive item.'}
        </Notice>
      ) : null}

      {bulkRedrive.isSuccess ? (
        <Notice
          tone="ok"
          title="Dead-letter items queued"
          onDismiss={() => bulkRedrive.reset()}
        >
          {formatNumber(bulkRedrive.data.redriven)} items were reset to pending.
        </Notice>
      ) : bulkRedrive.isError ? (
        <Notice tone="fail" title="Bulk redrive failed">
          {(bulkRedrive.error as Error)?.message ??
            'Unable to redrive dead-letter items.'}
        </Notice>
      ) : null}

      <Toolbar>
        <Field label="state" htmlFor="catalog-items-state">
          <Select
            id="catalog-items-state"
            selectSize="sm"
            value={state}
            onChange={(event) =>
              setFilter({ state: event.target.value as CatalogItemsStateFilter })
            }
          >
            {STATES.map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="object_id" htmlFor="catalog-items-object-id">
          <Input
            id="catalog-items-object-id"
            inputSize="sm"
            value={objectID}
            placeholder="substring"
            onChange={(event) => setFilter({ object_id: event.target.value })}
          />
        </Field>
        <Field label="limit" htmlFor="catalog-items-limit">
          <Select
            id="catalog-items-limit"
            selectSize="sm"
            value={String(limit)}
            onChange={(event) => setFilter({ limit: Number(event.target.value) })}
          >
            {[25, 50, 100, 250].map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </Select>
        </Field>
      </Toolbar>

      {items.isLoading ? (
        <LoadingState rows={7} label="loading catalog items" />
      ) : rows.length === 0 && !items.isError ? (
        <EmptyState
          title="No catalog items match"
          description="Adjust the state or object_id filter to broaden the result set."
        />
      ) : (
        <>
          <Table>
            <Thead>
              <Tr>
                <Th>state</Th>
                <Th>object_id</Th>
                <Th>strategy</Th>
                <Th align="right">attempts</Th>
                <Th>last error</Th>
                <Th>updated</Th>
                <Th align="right">actions</Th>
              </Tr>
            </Thead>
            <Tbody>
              {rows.map((item) => (
                <Tr key={item.id}>
                  <Td>
                    <StatusToken state={stateToken(item.state)} title={item.state} />
                  </Td>
                  <Td mono>
                    <Link
                      to={`${paths.nsCatalogItem(name, String(item.id))}${location.search}`}
                      className="hover:text-accent"
                    >
                      {item.object_id}
                    </Link>
                  </Td>
                  <Td mono>
                    {item.strategy_id && item.strategy_version
                      ? `${item.strategy_id}@${item.strategy_version}`
                      : 'none'}
                  </Td>
                  <Td mono align="right">
                    {formatNumber(item.attempt_count)}
                  </Td>
                  <Td className="max-w-72 truncate" title={item.last_error}>
                    {item.last_error || '—'}
                  </Td>
                  <Td mono>{formatRelative(item.updated_at)}</Td>
                  <Td align="right">
                    {canRedrive(item.state) ? (
                      <Button
                        variant="ghost"
                        size="xs"
                        onClick={() => setRedriveTarget(item)}
                      >
                        redrive
                      </Button>
                    ) : (
                      <span className="font-mono text-xs text-muted">—</span>
                    )}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>

          <Pagination
            offset={offset}
            limit={limit}
            total={items.data?.total}
            onOffsetChange={(next) => setFilter({ offset: next })}
          />
        </>
      )}

      <ConfirmDialog
        open={redriveTarget !== null}
        title="Redrive catalog item?"
        description={
          redriveTarget
            ? `Reset ${redriveTarget.object_id} to pending and enqueue it for embedding.`
            : undefined
        }
        confirmLabel="Redrive"
        loading={redrive.isPending}
        onConfirm={() => {
          if (!redriveTarget) return
          redrive.mutate(
            { namespace: name, id: redriveTarget.id },
            { onSettled: () => setRedriveTarget(null) },
          )
        }}
        onCancel={() => setRedriveTarget(null)}
      />

      <ConfirmDialog
        open={showBulkConfirm}
        title="Redrive dead-letter catalog items?"
        description={`Reset ${formatNumber(deadLetterCount)} dead-letter items to pending and enqueue them for embedding.`}
        confirmLabel="Redrive deadletter"
        loading={bulkRedrive.isPending}
        onConfirm={() =>
          bulkRedrive.mutate(name, {
            onSettled: () => setShowBulkConfirm(false),
          })
        }
        onCancel={() => setShowBulkConfirm(false)}
      />

      <Outlet />
    </PageShell>
  )
}
