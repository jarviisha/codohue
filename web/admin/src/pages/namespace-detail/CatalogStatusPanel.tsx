import { Link } from 'react-router-dom'
import { Badge, KeyValueList, KeyValueRow, MetricTile, Panel } from '../../components/ui'
import { useCatalogConfig } from '../../hooks/useCatalogConfig'
import { useLastReembedRun } from '../../hooks/useCatalogReembed'
import { ApiError } from '../../services/api'
import { formatCount, formatDateTimeShort, formatDurationMs } from '../../utils/format'

// CatalogStatusPanel renders the operational snapshot for a namespace's
// catalog auto-embedding (T043):
//
//   • Backlog counts          — pending / in_flight / embedded / failed /
//                               dead_letter / stream_len
//   • Active strategy         — id + version + dim + max_attempts +
//                               max_content_bytes
//   • Last re-embed run       — id, status, started/completed timestamps,
//                               processed count
//
// Polls every 10s while the namespace is open so the operator sees the
// embedder drain backlog without manually refreshing.
export default function CatalogStatusPanel({ namespace }: { namespace: string }) {
  const { data, isLoading, error } = useCatalogConfig(namespace)
  const { data: lastRun } = useLastReembedRun(namespace)

  if (error instanceof ApiError && error.status === 503) {
    return null // CatalogConfigForm renders the 503 notice — avoid double display.
  }

  if (isLoading || !data) {
    return (
      <Panel title="Catalog Status">
        <p className="m-0 text-sm text-muted">Loading status…</p>
      </Panel>
    )
  }

  const { catalog, backlog } = data
  const lastRunInProgress = lastRun != null && lastRun.completed_at == null

  return (
    <Panel
      title="Catalog Status"
      actions={
        <Link
          to={`/namespaces/${encodeURIComponent(namespace)}/catalog/items`}
          className="text-sm font-medium text-accent hover:underline"
        >
          Browse items →
        </Link>
      }
    >
      <div className="flex flex-col gap-5">
        {!catalog.enabled && (
          <p className="m-0 text-sm text-muted">
            Auto-embedding is currently <strong>disabled</strong>. Enable it via the
            form above to start ingesting raw catalog items.
          </p>
        )}

        <section className="flex flex-col gap-2">
          <h4 className="m-0 text-xs font-semibold uppercase tracking-[0.06em] text-muted">
            Backlog
          </h4>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
            <MetricTile label="pending" value={formatCount(backlog.pending)} className="bg-subtle" />
            <MetricTile label="in flight" value={formatCount(backlog.in_flight)} className="bg-subtle" />
            <MetricTile label="embedded" value={formatCount(backlog.embedded)} className="bg-subtle" />
            <MetricTile label="failed" value={formatCount(backlog.failed)} className="bg-subtle" />
            <MetricTile
              label="dead-letter"
              value={formatCount(backlog.dead_letter)}
              className={backlog.dead_letter > 0 ? 'bg-danger-bg' : 'bg-subtle'}
            />
            <MetricTile label="stream len" value={formatCount(backlog.stream_len)} className="bg-subtle" />
          </div>
        </section>

        {catalog.enabled && (
          <section className="flex flex-col gap-2">
            <h4 className="m-0 text-xs font-semibold uppercase tracking-[0.06em] text-muted">
              Active Strategy
            </h4>
            <KeyValueList>
              <KeyValueRow
                label="Strategy"
                value={
                  catalog.strategy_id
                    ? `${catalog.strategy_id} / ${catalog.strategy_version ?? ''}`
                    : '—'
                }
              />
              <KeyValueRow label="Embedding dim" value={String(catalog.embedding_dim)} />
              <KeyValueRow label="Max retry attempts" value={String(catalog.max_attempts)} />
              <KeyValueRow
                label="Max content bytes"
                value={`${formatCount(catalog.max_content_bytes)} (${(catalog.max_content_bytes / 1024).toFixed(1)} KiB)`}
              />
              <KeyValueRow
                label="Config updated"
                value={formatDateTimeShort(catalog.updated_at)}
              />
            </KeyValueList>
          </section>
        )}

        {lastRun != null && (
          <section className="flex flex-col gap-2">
            <h4 className="m-0 text-xs font-semibold uppercase tracking-[0.06em] text-muted">
              Last re-embed run
            </h4>
            <div className="flex flex-wrap items-center gap-3">
              <span className="text-sm font-medium text-primary tabular-nums">
                #{lastRun.id}
              </span>
              {lastRunInProgress ? (
                <Badge tone="warning" dot>In progress</Badge>
              ) : lastRun.success ? (
                <Badge tone="success" dot>Succeeded</Badge>
              ) : (
                <Badge tone="danger" dot>Failed</Badge>
              )}
            </div>
            <KeyValueList>
              <KeyValueRow label="Started" value={formatDateTimeShort(lastRun.started_at)} />
              <KeyValueRow
                label="Completed"
                value={lastRun.completed_at ? formatDateTimeShort(lastRun.completed_at) : '—'}
              />
              <KeyValueRow label="Duration" value={formatDurationMs(lastRun.duration_ms)} />
              <KeyValueRow
                label="Items processed"
                value={formatCount(lastRun.subjects_processed)}
              />
            </KeyValueList>
          </section>
        )}
      </div>
    </Panel>
  )
}
