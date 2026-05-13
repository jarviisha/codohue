import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge, Button, KeyValueList, KeyValueRow, Notice, Panel } from '../../components/ui'
import {
  useTriggerCatalogReEmbed,
  useLastReembedRun,
} from '../../hooks/useCatalogReembed'
import { ApiError } from '../../services/api'
import { formatDateTimeShort, formatDurationMs } from '../../utils/format'

// CatalogOpsPanel renders the operator-side controls for a namespace's
// catalog auto-embedding lifecycle (US3). Currently exposes:
//
//   • Trigger namespace-wide re-embed (T055)
//   • Last re-embed run status (in-progress / completed)
//   • Quick link to the catalog items browser (T056 / T057)
//
// The panel intentionally degrades gracefully when the catalog feature is
// not enabled in this deployment (503 from the API → operator sees the
// not-wired notice).
export default function CatalogOpsPanel({ namespace }: { namespace: string }) {
  const trigger = useTriggerCatalogReEmbed(namespace)
  const { data: lastRun } = useLastReembedRun(namespace)
  const [error, setError] = useState<string>('')

  const lastRunInProgress = lastRun != null && lastRun.completed_at == null

  async function handleTrigger() {
    setError('')
    try {
      await trigger.mutateAsync()
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(`${err.code}: ${err.message}`)
      } else {
        setError(err instanceof Error ? err.message : 'Re-embed trigger failed')
      }
    }
  }

  return (
    <Panel
      title="Catalog Auto-Embedding"
      actions={
        <Link
          to={`/namespaces/${encodeURIComponent(namespace)}/catalog/items`}
          className="text-sm font-medium text-accent hover:underline"
        >
          Browse items →
        </Link>
      }
    >
      <div className="space-y-4">
        {error && (
          <Notice tone="danger" onDismiss={() => setError('')}>
            {error}
          </Notice>
        )}

        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="primary"
            disabled={trigger.isPending || lastRunInProgress}
            onClick={handleTrigger}
          >
            {trigger.isPending ? 'Triggering…' : 'Re-embed namespace'}
          </Button>

          {lastRunInProgress && (
            <Badge tone="warning" dot>Re-embed in progress</Badge>
          )}
          {!lastRunInProgress && lastRun?.success && (
            <Badge tone="success" dot>Last re-embed succeeded</Badge>
          )}
          {!lastRunInProgress && lastRun != null && !lastRun.success && (
            <Badge tone="danger" dot>Last re-embed failed</Badge>
          )}
          {lastRun == null && (
            <span className="text-xs text-muted">No re-embed has been triggered yet.</span>
          )}
        </div>

        {lastRun != null && (
          <KeyValueList>
            <KeyValueRow label="Last run id" value={`#${lastRun.id}`} />
            <KeyValueRow label="Started at" value={formatDateTimeShort(lastRun.started_at)} />
            <KeyValueRow
              label="Completed at"
              value={lastRun.completed_at ? formatDateTimeShort(lastRun.completed_at) : '—'}
            />
            <KeyValueRow label="Duration" value={formatDurationMs(lastRun.duration_ms)} />
            <KeyValueRow
              label="Items processed"
              value={lastRun.subjects_processed.toLocaleString()}
            />
            {lastRun.error_message && lastRun.error_message.startsWith('reembed:') && (
              <KeyValueRow
                label="Target strategy"
                value={lastRun.error_message.replace(/^reembed:/, '')}
              />
            )}
            {lastRun.error_message && !lastRun.error_message.startsWith('reembed:') && (
              <KeyValueRow label="Error" value={lastRun.error_message} />
            )}
          </KeyValueList>
        )}

        <p className="m-0 text-xs text-muted">
          Re-embed resets every catalog item whose <code>strategy_version</code> differs from
          the namespace's currently active version back to <code>pending</code> and re-publishes
          them to the embed stream. Items that are already up-to-date are left alone.
        </p>
      </div>
    </Panel>
  )
}
