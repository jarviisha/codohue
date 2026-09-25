import { useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge, Button, ProgressBar, Stack } from '@astryxdesign/core'
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
      <div className="bg-popover border border-border rounded shadow-lg px-4 py-3">
        <Stack gap={4} direction="horizontal" align="center" wrap="wrap">
          <Stack gap={4} direction="horizontal" align="center">
            <Badge variant="info" label="re-embed" />
            <span className="text-primary text-sm">
              {runs.length} run{runs.length === 1 ? '' : 's'} in flight
            </span>
          </Stack>
          <Stack gap={4} direction="horizontal" align="center" wrap="wrap">
            {runs.map((r) => (
              <ProgressChip key={r.id} run={r} />
            ))}
          </Stack>
          <Button
            size="sm"
            variant="ghost"
            
            onClick={() => setActive({})}
            aria-label="Dismiss overlay" label="Hide" />
        </Stack>
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
      className="block text-primary no-underline hover:underline"
    >
      <Stack gap={6}>
        <Stack gap={4} direction="horizontal" align="center">
          <span className="text-sm font-medium">
            #{run.id} <NamespaceTag name={run.namespace} />
          </span>
          {hasProgress && (
            <span className="text-secondary text-xs tabular-nums">
              {run.processed!.toLocaleString()} / {run.total!.toLocaleString()} ({pct}%)
            </span>
          )}
        </Stack>
        {hasProgress && (
          <ProgressBar
            label={`Re-embed #${run.id} progress`}
            value={run.processed!}
            max={run.total!}
            isLabelHidden
          />
        )}
      </Stack>
    </Link>
  )
}
