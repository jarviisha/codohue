import { KeyValueList, Panel, StatusToken } from '@/components/ui'
import { formatNumber, formatRelative } from '@/utils/format'
import { useCatalogContext } from './catalogContext'

// Status tab — read-only snapshot of the catalog feature for this namespace.
// Mutations (config edits, re-embed, item redrive) live on sibling routes.
export default function CatalogStatusPage() {
  const { data } = useCatalogContext()

  return (
    <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
      <Panel title="status">
        <KeyValueList
          rows={[
            {
              label: 'enabled',
              value: (
                <StatusToken
                  state={data.catalog.enabled ? 'ok' : 'idle'}
                  label={data.catalog.enabled ? 'enabled' : 'disabled'}
                />
              ),
            },
            {
              label: 'strategy',
              value:
                data.catalog.strategy_id && data.catalog.strategy_version
                  ? `${data.catalog.strategy_id}@${data.catalog.strategy_version}`
                  : 'none',
            },
            { label: 'embedding_dim', value: data.catalog.embedding_dim.toString() },
            { label: 'updated', value: formatRelative(data.catalog.updated_at) },
          ]}
        />
      </Panel>

      <Panel title="backlog">
        <KeyValueList
          rows={[
            {
              label: 'pending',
              value: (
                <StatusToken
                  state={data.backlog.pending > 0 ? 'warn' : 'ok'}
                  label={formatNumber(data.backlog.pending)}
                />
              ),
            },
            { label: 'in_flight', value: formatNumber(data.backlog.in_flight) },
            { label: 'embedded', value: formatNumber(data.backlog.embedded) },
            {
              label: 'failed',
              value: (
                <StatusToken
                  state={data.backlog.failed > 0 ? 'fail' : 'ok'}
                  label={formatNumber(data.backlog.failed)}
                />
              ),
            },
            {
              label: 'dead_letter',
              value: (
                <StatusToken
                  state={data.backlog.dead_letter > 0 ? 'fail' : 'ok'}
                  label={formatNumber(data.backlog.dead_letter)}
                />
              ),
            },
            { label: 'stream_len', value: formatNumber(data.backlog.stream_len) },
          ]}
        />
      </Panel>
    </div>
  )
}
