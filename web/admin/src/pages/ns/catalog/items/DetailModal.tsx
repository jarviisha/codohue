import { useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  Button,
  CodeBlock,
  ConfirmDialog,
  KeyValueList,
  LoadingState,
  Modal,
  Notice,
  StatusToken,
  useRegisterCommand,
} from '@/components/ui'
import {
  useCatalogItem,
  useDeleteCatalogItem,
  useRedriveCatalogItem,
} from '@/services/catalog'
import { paths } from '@/routes/path'
import { formatNumber, formatTimestamp } from '@/utils/format'
import { canRedrive, stateToken } from './helpers'

// Modal route for /ns/:name/catalog/items/:id. Wraps ItemsPage's <Outlet> so
// the underlying list stays mounted while inspecting one row.
export default function CatalogItemDetailModal() {
  const { name = '', id = '' } = useParams<{ name: string; id: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const item = useCatalogItem(name, id)
  const redrive = useRedriveCatalogItem()
  const deleteItem = useDeleteCatalogItem()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [showRedriveConfirm, setShowRedriveConfirm] = useState(false)

  const close = () => {
    navigate({ pathname: paths.nsCatalogItems(name), search: location.search })
  }

  useRegisterCommand(
    `ns.${name}.catalog.item.close`,
    `Close catalog item ${id}`,
    close,
    name,
  )

  const data = item.data
  const metadataJson = JSON.stringify(data?.metadata ?? {}, null, 2)

  return (
    <>
      <Modal
        open
        onClose={close}
        size="lg"
        title={data ? `catalog item ${data.id}` : 'catalog item'}
        footer={
          <>
            <Button variant="ghost" onClick={close}>Close</Button>
            <Button
              variant="secondary"
              disabled={!data || !canRedrive(data.state)}
              onClick={() => setShowRedriveConfirm(true)}
            >
              Redrive
            </Button>
            <Button
              variant="danger"
              disabled={!data}
              onClick={() => setShowDeleteConfirm(true)}
            >
              Delete
            </Button>
          </>
        }
      >
        {item.isLoading ? (
          <LoadingState rows={6} label="loading catalog item" />
        ) : item.isError || !data ? (
          <Notice tone="fail" title="Failed to load catalog item">
            {(item.error as Error)?.message ?? 'Catalog item not found.'}
          </Notice>
        ) : (
          <div className="flex flex-col gap-4">
            {redrive.isError ? (
              <Notice tone="fail" title="Redrive failed">
                {(redrive.error as Error)?.message ?? 'Unable to redrive item.'}
              </Notice>
            ) : null}

            {deleteItem.isError ? (
              <Notice tone="fail" title="Delete failed">
                {(deleteItem.error as Error)?.message ?? 'Unable to delete catalog item.'}
              </Notice>
            ) : null}

            <KeyValueList
              rows={[
                {
                  label: 'state',
                  value: (
                    <StatusToken
                      state={stateToken(data.state)}
                      title={data.state}
                      label={data.state}
                    />
                  ),
                },
                { label: 'object_id', value: data.object_id },
                {
                  label: 'strategy',
                  value:
                    data.strategy_id && data.strategy_version
                      ? `${data.strategy_id}@${data.strategy_version}`
                      : 'none',
                },
                { label: 'attempts', value: formatNumber(data.attempt_count) },
                {
                  label: 'vector',
                  value: data.vector
                    ? `${data.vector.collection} #${data.vector.numeric_id} · ${data.vector.dim}d`
                    : 'not indexed',
                },
                { label: 'embedded_at', value: formatTimestamp(data.embedded_at) },
                { label: 'created_at', value: formatTimestamp(data.created_at) },
                { label: 'updated_at', value: formatTimestamp(data.updated_at) },
                { label: 'last_error', value: data.last_error || '—' },
              ]}
            />

            <div>
              <h3 className="mb-2 font-mono text-xs uppercase tracking-[0.04em] text-secondary">
                content
              </h3>
              <CodeBlock language="text" copyable maxHeight="16rem">
                {data.content}
              </CodeBlock>
            </div>

            {data.vector ? (
              <div>
                <h3 className="mb-2 font-mono text-xs uppercase tracking-[0.04em] text-secondary">
                  dense vector
                </h3>
                <CodeBlock language="json" copyable maxHeight="16rem">
                  {JSON.stringify(data.vector.values, null, 2)}
                </CodeBlock>
              </div>
            ) : null}

            <div>
              <h3 className="mb-2 font-mono text-xs uppercase tracking-[0.04em] text-secondary">
                metadata
              </h3>
              <CodeBlock language="json" copyable maxHeight="12rem">
                {metadataJson}
              </CodeBlock>
            </div>
          </div>
        )}
      </Modal>

      <ConfirmDialog
        open={showRedriveConfirm}
        title="Redrive catalog item?"
        description={
          data
            ? `Reset ${data.object_id} to pending and enqueue it for embedding.`
            : undefined
        }
        confirmLabel="Redrive"
        loading={redrive.isPending}
        onConfirm={() => {
          if (!data) return
          redrive.mutate(
            { namespace: name, id: data.id },
            {
              onSuccess: () => {
                setShowRedriveConfirm(false)
                void item.refetch()
              },
            },
          )
        }}
        onCancel={() => setShowRedriveConfirm(false)}
      />

      <ConfirmDialog
        open={showDeleteConfirm}
        title="Delete catalog item?"
        description={
          data
            ? `Delete ${data.object_id} from catalog_items. This does not delete the source object.`
            : undefined
        }
        confirmLabel="Delete"
        destructive
        loading={deleteItem.isPending}
        onConfirm={() => {
          if (!data) return
          deleteItem.mutate(
            { namespace: name, id: data.id },
            { onSuccess: close },
          )
        }}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </>
  )
}
