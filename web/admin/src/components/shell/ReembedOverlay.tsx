import { useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge, Button, Inline, Stack } from '@jarviisha/davinci-react-ui'
import { useServerStream } from '@/services/stream'
import NamespaceTag from '@/components/NamespaceTag'

type RunningReembed = {
  id: number
  namespace: string
  startedAt: string
  processed?: number
  total?: number
}

/**
 * ReembedOverlay shows a sticky banner at the bottom of the viewport listing
 * every catalog re-embed run currently in flight. Sources:
 *
 *   - Global SSE `/stream` provides `batch_run.started` / `.completed` /
 *     `.cancelled` events; we filter on `kind=reembed` and maintain a small
 *     in-memory map keyed by run id.
 *   - `catalog.reembed_progress` ticks (one per watcher poll per open run)
 *     fill in processed / total so each chip renders a tiny progress bar.
 *   - The banner is hidden when the map is empty (idle path renders nothing).
 *
 * Click a chip → `/batch-runs/{id}` for full progress + log lines.
 */
export default function ReembedOverlay() {
  const [active, setActive] = useState<Record<number, RunningReembed>>({})

  const dismiss = useCallback(
    (id: number) =>
      setActive((m) => {
        const { [id]: _gone, ...rest } = m
        void _gone
        return rest
      }),
    [],
  )

  useServerStream(
    '/api/admin/v1/stream',
    useMemo(
      () => ({
        'batch_run.started': (data: unknown) => {
          const d = data as { id?: number; namespace?: string; kind?: string }
          if (d.kind !== 'reembed' || d.id == null || !d.namespace) return
          setActive((m) => ({
            ...m,
            [d.id!]: {
              id: d.id!,
              namespace: d.namespace!,
              startedAt: new Date().toISOString(),
            },
          }))
        },
        'batch_run.completed': (data: unknown) => {
          const d = data as { id?: number }
          if (d.id != null) dismiss(d.id)
        },
        'batch_run.cancelled': (data: unknown) => {
          const d = data as { id?: number }
          if (d.id != null) dismiss(d.id)
        },
        // Catalog watcher emits one of these per open run per 5s tick. The
        // first one may arrive BEFORE batch_run.started (different event
        // loops), so we upsert defensively rather than skip-unknown.
        'catalog.reembed_progress': (data: unknown) => {
          const d = data as {
            batch_run_id?: number
            namespace?: string
            processed?: number
            total?: number
          }
          if (d.batch_run_id == null || !d.namespace) return
          setActive((m) => {
            const existing = m[d.batch_run_id!]
            return {
              ...m,
              [d.batch_run_id!]: {
                id: d.batch_run_id!,
                namespace: d.namespace!,
                startedAt: existing?.startedAt ?? new Date().toISOString(),
                processed: d.processed,
                total: d.total,
              },
            }
          })
        },
      }),
      [dismiss],
    ),
  )

  const runs = Object.values(active)
  if (runs.length === 0) return null

  return (
    <div
      role="status"
      className="fixed bottom-4 left-1/2 -translate-x-1/2 z-50 max-w-3xl"
    >
      <div className="bg-surface-raised border border-default rounded shadow-lg px-4 py-3">
        <Inline align="center" wrap>
          <Inline align="center">
            <Badge variant="primary">re-embed</Badge>
            <span className="text-foreground text-sm">
              {runs.length} run{runs.length === 1 ? '' : 's'} in flight
            </span>
          </Inline>
          <Inline align="center" wrap>
            {runs.map((r) => (
              <ProgressChip key={r.id} run={r} />
            ))}
          </Inline>
          <Button
            size="sm"
            variant="ghost"
            tone="neutral"
            onClick={() => setActive({})}
            aria-label="Dismiss overlay"
          >
            Hide
          </Button>
        </Inline>
      </div>
    </div>
  )
}

function ProgressChip({ run }: { run: RunningReembed }) {
  const hasProgress = run.processed != null && run.total != null && run.total > 0
  const pct = hasProgress ? Math.min(100, Math.round((run.processed! / run.total!) * 100)) : null
  return (
    <Link
      to={`/batch-runs/${run.id}`}
      className="block text-foreground no-underline hover:underline"
    >
      <Stack>
        <Inline align="center">
          <span className="text-sm font-medium">
            #{run.id} <NamespaceTag name={run.namespace} />
          </span>
          {hasProgress && (
            <span className="text-foreground-subtle text-xs tabular-nums">
              {run.processed!.toLocaleString()} / {run.total!.toLocaleString()} ({pct}%)
            </span>
          )}
        </Inline>
        {hasProgress && (
          <div className="h-1 w-40 bg-surface-sunken rounded overflow-hidden">
            <div
              className="h-full bg-primary transition-all duration-300"
              style={{ width: `${pct}%` }}
            />
          </div>
        )}
      </Stack>
    </Link>
  )
}
