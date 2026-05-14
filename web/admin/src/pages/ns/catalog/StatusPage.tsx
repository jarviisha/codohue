import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Button,
  ConfirmDialog,
  KeyValueList,
  Notice,
  Panel,
  StatusToken,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
} from '@/components/ui'
import type { StatusState } from '@/components/ui'
import {
  useBulkRedriveDeadletter,
  useCatalogItems,
} from '@/services/catalog'
import type {
  CatalogBacklog,
  CatalogReEmbedSummary,
  NamespaceCatalogResponse,
} from '@/services/catalog'
import { paths } from '@/routes/path'
import { formatNumber, formatRelative } from '@/utils/format'
import { useCatalogContext } from './catalogContext'

const STALE_EMBED_THRESHOLD_MS = 60 * 60 * 1000 // 1 hour

// Threshold rules (kept conservative so the operator only sees red when
// action is actually required):
//   pending / in_flight / failed → idle (transient, will drain via retry)
//   dead_letter > 0 → fail (exhausted retries, needs human action)
//   embedded total → ok
function backlogTotal(backlog: CatalogBacklog) {
  return (
    backlog.pending +
    backlog.in_flight +
    backlog.embedded +
    backlog.failed +
    backlog.dead_letter
  )
}

function inFlightToken(n: number): StatusState {
  return n > 0 ? 'run' : 'idle'
}

interface Health {
  tone: 'fail' | 'warn' | 'run'
  title: string
  message: string
}

// Single point of truth for the derived health summary surfaced at the top
// of the Status tab. Returns null when nothing needs the operator's eyes —
// no banner means the catalog is healthy.
function deriveHealth(data: NamespaceCatalogResponse): Health | null {
  const reEmbed = data.last_re_embed
  if (reEmbed?.status === 'failed') {
    return {
      tone: 'fail',
      title: `Last re-embed failed (batch #${reEmbed.batch_run_id})`,
      message: reEmbed.error_message || 'Re-embed run ended with an unspecified error.',
    }
  }
  if (data.backlog.dead_letter > 0) {
    return {
      tone: 'fail',
      title: `${formatNumber(data.backlog.dead_letter)} dead-letter items need attention`,
      message: 'Items have exhausted their retry budget. Redrive or delete them.',
    }
  }
  if (reEmbed?.status === 'running') {
    return {
      tone: 'run',
      title: `Re-embed in progress (batch #${reEmbed.batch_run_id})`,
      message: `Started ${formatRelative(reEmbed.started_at)}.`,
    }
  }
  if (!data.catalog.enabled) return null

  const last = data.last_embedded_at ? new Date(data.last_embedded_at).getTime() : 0
  const ageMs = last ? Date.now() - last : Infinity

  if (!last && data.backlog.embedded === 0) {
    return {
      tone: 'warn',
      title: 'No catalog items embedded yet',
      message: 'Either no content has been ingested or the embedder has not consumed it.',
    }
  }
  if (data.backlog.pending > 0 && ageMs > STALE_EMBED_THRESHOLD_MS) {
    return {
      tone: 'warn',
      title: 'Pipeline may be stalled',
      message: `${formatNumber(data.backlog.pending)} items pending and no embed in the last hour.`,
    }
  }
  return null
}

function reembedToken(status: CatalogReEmbedSummary['status']): StatusState {
  if (status === 'running') return 'run'
  if (status === 'failed') return 'fail'
  return 'ok'
}

export default function CatalogStatusPage() {
  const { name = '' } = useParams<{ name: string }>()
  const { data } = useCatalogContext()
  const bulkRedrive = useBulkRedriveDeadletter()
  const [showBulkConfirm, setShowBulkConfirm] = useState(false)

  const deadLetterCount = data.backlog.dead_letter
  const recentErrors = useCatalogItems({
    namespace: name,
    state: 'dead_letter',
    limit: 5,
    offset: 0,
  })
  const errorRows = recentErrors.data?.items ?? []
  const total = backlogTotal(data.backlog)
  const health = deriveHealth(data)
  const reEmbed = data.last_re_embed

  return (
    <div className="flex flex-col gap-4">
      {health ? (
        <Notice tone={health.tone === 'run' ? 'warn' : health.tone} title={health.title}>
          {health.message}
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
          {(bulkRedrive.error as Error)?.message ?? 'Unable to redrive dead-letter items.'}
        </Notice>
      ) : null}

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
              {
                label: 'last embed',
                value: data.last_embedded_at ? formatRelative(data.last_embedded_at) : '—',
              },
              { label: 'config updated', value: formatRelative(data.catalog.updated_at) },
            ]}
          />
        </Panel>

        <Panel
          title="backlog"
          actions={
            <Button
              variant="primary"
              size="sm"
              disabled={deadLetterCount === 0}
              loading={bulkRedrive.isPending}
              onClick={() => setShowBulkConfirm(true)}
            >
              Redrive deadletter ({formatNumber(deadLetterCount)})
            </Button>
          }
        >
          <KeyValueList
            rows={[
              { label: 'total', value: formatNumber(total) },
              { label: 'embedded', value: formatNumber(data.backlog.embedded) },
              { label: 'pending', value: formatNumber(data.backlog.pending) },
              {
                label: 'in_flight',
                value: (
                  <StatusToken
                    state={inFlightToken(data.backlog.in_flight)}
                    label={formatNumber(data.backlog.in_flight)}
                  />
                ),
              },
              { label: 'failed', value: formatNumber(data.backlog.failed) },
              {
                label: 'dead_letter',
                value: (
                  <StatusToken
                    state={deadLetterCount > 0 ? 'fail' : 'ok'}
                    label={formatNumber(deadLetterCount)}
                  />
                ),
              },
              {
                label: (
                  <span title="Items currently buffered in the Redis Stream (catalog:embed:{ns}) waiting for the embedder worker to consume.">
                    stream queue
                  </span>
                ),
                value: formatNumber(data.backlog.stream_len),
              },
            ]}
          />
        </Panel>
      </div>

      {reEmbed ? (
        <Panel title="last re-embed">
          <KeyValueList
            rows={[
              {
                label: 'status',
                value: (
                  <StatusToken
                    state={reembedToken(reEmbed.status)}
                    label={`${reEmbed.status} · #${reEmbed.batch_run_id}`}
                  />
                ),
              },
              {
                label: 'target strategy',
                value:
                  reEmbed.strategy_id && reEmbed.strategy_version
                    ? `${reEmbed.strategy_id}@${reEmbed.strategy_version}`
                    : '—',
              },
              { label: 'started', value: formatRelative(reEmbed.started_at) },
              ...(reEmbed.completed_at
                ? [{ label: 'completed', value: formatRelative(reEmbed.completed_at) }]
                : []),
              ...(reEmbed.duration_ms
                ? [{ label: 'duration', value: `${formatNumber(Math.round(reEmbed.duration_ms / 1000))}s` }]
                : []),
              { label: 'processed', value: formatNumber(reEmbed.processed_items) },
              ...(reEmbed.error_message
                ? [{ label: 'error', value: reEmbed.error_message }]
                : []),
            ]}
          />
        </Panel>
      ) : null}

      {deadLetterCount > 0 ? (
        <Panel
          title="recent dead-letter items"
          actions={
            <Link
              to={`${paths.nsCatalogItems(name)}?state=dead_letter`}
              className="font-mono text-xs uppercase tracking-[0.04em] text-secondary hover:text-primary"
            >
              View all →
            </Link>
          }
        >
          {errorRows.length === 0 ? (
            <p className="text-sm text-muted">Loading…</p>
          ) : (
            <Table>
              <Thead>
                <Tr>
                  <Th>object_id</Th>
                  <Th>last error</Th>
                  <Th align="right">attempts</Th>
                  <Th>updated</Th>
                </Tr>
              </Thead>
              <Tbody>
                {errorRows.map((item) => (
                  <Tr key={item.id}>
                    <Td mono>
                      <Link
                        to={paths.nsCatalogItem(name, String(item.id))}
                        className="hover:text-accent"
                      >
                        {item.object_id}
                      </Link>
                    </Td>
                    <Td className="max-w-xl truncate" title={item.last_error}>
                      {item.last_error || '—'}
                    </Td>
                    <Td mono align="right">{formatNumber(item.attempt_count)}</Td>
                    <Td mono>{formatRelative(item.updated_at)}</Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
          )}
        </Panel>
      ) : null}

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
    </div>
  )
}
