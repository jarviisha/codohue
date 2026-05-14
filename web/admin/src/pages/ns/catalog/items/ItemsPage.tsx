import { useState } from 'react'
import { Link, Outlet, useLocation, useParams } from 'react-router-dom'
import {
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  LoadingState,
  Notice,
  Pagination,
  Panel,
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
} from '@/components/ui'
import type {
  CatalogItemSummary,
  CatalogItemsStateFilter,
} from '@/services/catalog'
import {
  useBulkRedriveDeadletter,
  useCatalogItems,
  useRedriveCatalogItem,
} from '@/services/catalog'
import { paths } from '@/routes/path'
import { formatNumber, formatRelative } from '@/utils/format'
import { useCatalogContext } from '@/pages/ns/catalog/catalogContext'
import { ITEM_STATES, canRedrive, stateToken } from './helpers'
import { useItemsFilter } from './useItemsFilter'

// Items tab — paginated browse of catalog items with state/object_id filters
// and single + bulk redrive actions. The :id child route renders DetailModal.
export default function CatalogItemsPage() {
  const { name = '' } = useParams<{ name: string }>()
  const location = useLocation()
  const { data: catalog } = useCatalogContext()
  const { filter, setFilter } = useItemsFilter()

  const items = useCatalogItems({
    namespace: name,
    state: filter.state,
    object_id: filter.objectID,
    limit: filter.limit,
    offset: filter.offset,
  })
  const redrive = useRedriveCatalogItem()
  const bulkRedrive = useBulkRedriveDeadletter()
  const [redriveTarget, setRedriveTarget] = useState<CatalogItemSummary | null>(null)
  const [showBulkConfirm, setShowBulkConfirm] = useState(false)

  const rows = items.data?.items ?? []
  const deadLetterCount = catalog.backlog.dead_letter

  useRegisterCommand(
    `ns.${name}.catalog.items.refresh`,
    `Refresh ${name} catalog items`,
    () => void items.refetch(),
    name,
  )
  useRegisterCommand(
    `ns.${name}.catalog.items.redriveDeadletter`,
    `Redrive ${name} dead-letter catalog items`,
    () => setShowBulkConfirm(true),
    name,
  )

  return (
    <Panel
      title="items"
      actions={
        <>
          <Button
            variant="ghost"
            size="sm"
            loading={items.isFetching && !items.isLoading}
            onClick={() => void items.refetch()}
          >
            Refresh
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
    >
      <div className="flex flex-col gap-4">
        {items.isError ? (
          <Notice tone="fail" title="Failed to load catalog items">
            {(items.error as Error)?.message ?? 'Unable to load catalog items.'}
          </Notice>
        ) : null}

        {redrive.isSuccess ? (
          <Notice
            tone="ok"
            title="Catalog item queued"
            onDismiss={() => redrive.reset()}
          >
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
              value={filter.state}
              onChange={(event) =>
                setFilter({ state: event.target.value as CatalogItemsStateFilter })
              }
            >
              {ITEM_STATES.map((value) => (
                <option key={value} value={value}>{value}</option>
              ))}
            </Select>
          </Field>
          <Field label="object_id" htmlFor="catalog-items-object-id">
            <Input
              id="catalog-items-object-id"
              inputSize="sm"
              value={filter.objectID}
              placeholder="substring"
              onChange={(event) => setFilter({ object_id: event.target.value })}
            />
          </Field>
          <Field label="limit" htmlFor="catalog-items-limit">
            <Select
              id="catalog-items-limit"
              selectSize="sm"
              value={String(filter.limit)}
              onChange={(event) => setFilter({ limit: Number(event.target.value) })}
            >
              {[25, 50, 100, 250].map((value) => (
                <option key={value} value={value}>{value}</option>
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
                  <Th>content</Th>
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
                    <Td className="max-w-md truncate" title={item.content_preview}>
                      {item.content_preview || '—'}
                    </Td>
                    <Td mono>
                      {item.strategy_id && item.strategy_version
                        ? `${item.strategy_id}@${item.strategy_version}`
                        : 'none'}
                    </Td>
                    <Td mono align="right">{formatNumber(item.attempt_count)}</Td>
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
              offset={filter.offset}
              limit={filter.limit}
              total={items.data?.total}
              onOffsetChange={(next) => setFilter({ offset: next })}
            />
          </>
        )}
      </div>

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
          bulkRedrive.mutate(name, { onSettled: () => setShowBulkConfirm(false) })
        }
        onCancel={() => setShowBulkConfirm(false)}
      />

      <Outlet />
    </Panel>
  )
}
