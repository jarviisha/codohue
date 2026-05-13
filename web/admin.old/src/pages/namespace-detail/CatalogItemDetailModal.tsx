import { Badge, KeyValueList, KeyValueRow, Modal } from '../../components/ui'
import type { CatalogItemDetail } from '../../types'
import { formatDateTimeShort } from '../../utils/format'

// CatalogItemDetailModal renders the full record for one catalog item: state,
// strategy, attempt count, content, metadata. Open via the items table row's
// object_id button. Read-only — re-drive / delete actions stay on the row.
export default function CatalogItemDetailModal({
  open,
  item,
  onClose,
}: {
  open: boolean
  item: CatalogItemDetail | null
  onClose: () => void
}) {
  return (
    <Modal open={open} onClose={onClose} title={item ? `Catalog item: ${item.object_id}` : 'Catalog item'}>
      {!item && (
        <p className="m-0 text-sm text-muted">Loading…</p>
      )}
      {item && (
        <div className="flex flex-col gap-4">
          <KeyValueList>
            <KeyValueRow label="ID" value={`#${item.id}`} />
            <KeyValueRow label="Namespace" value={item.namespace} />
            <KeyValueRow label="Object ID" value={item.object_id} />
            <KeyValueRow
              label="State"
              value={<Badge tone={stateTone(item.state)} dot>{item.state}</Badge>}
            />
            <KeyValueRow
              label="Strategy"
              value={
                item.strategy_id
                  ? `${item.strategy_id} / ${item.strategy_version ?? ''}`
                  : '—'
              }
            />
            <KeyValueRow label="Attempt count" value={String(item.attempt_count)} />
            <KeyValueRow
              label="Embedded at"
              value={item.embedded_at ? formatDateTimeShort(item.embedded_at) : '—'}
            />
            <KeyValueRow label="Created at" value={formatDateTimeShort(item.created_at)} />
            <KeyValueRow label="Updated at" value={formatDateTimeShort(item.updated_at)} />
            {item.last_error && (
              <KeyValueRow label="Last error" value={item.last_error} />
            )}
          </KeyValueList>

          <section className="flex flex-col gap-2">
            <h4 className="m-0 text-xs font-semibold uppercase tracking-[0.06em] text-muted">
              Content
            </h4>
            <pre className="m-0 max-h-64 overflow-auto rounded border border-default bg-subtle p-3 text-xs text-primary whitespace-pre-wrap wrap-break-word">
              {item.content}
            </pre>
          </section>

          {item.metadata && Object.keys(item.metadata).length > 0 && (
            <section className="flex flex-col gap-2">
              <h4 className="m-0 text-xs font-semibold uppercase tracking-[0.06em] text-muted">
                Metadata
              </h4>
              <pre className="m-0 max-h-64 overflow-auto rounded border border-default bg-subtle p-3 text-xs text-primary">
                {JSON.stringify(item.metadata, null, 2)}
              </pre>
            </section>
          )}
        </div>
      )}
    </Modal>
  )
}

function stateTone(state: CatalogItemDetail['state']): 'neutral' | 'success' | 'warning' | 'danger' | 'accent' {
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
