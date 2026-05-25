import { useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge, Button, Inline } from '@jarviisha/davinci-react-ui'
import { useServerStream } from '@/services/stream'

type RunningReembed = {
  id: number
  namespace: string
  startedAt: string
}

/**
 * ReembedOverlay shows a sticky banner at the bottom of the viewport listing
 * every catalog re-embed run currently in flight. Sources:
 *
 *   - Global SSE `/stream` provides `batch_run.started` / `.completed` /
 *     `.cancelled` events; we filter on `kind=reembed` and maintain a small
 *     in-memory map keyed by run id.
 *   - The banner is hidden when the map is empty (idle path renders nothing).
 *
 * Click a chip → `/batch-runs/{id}` for full progress. No polling here — the
 * detail page handles live phase + log streaming.
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
      }),
      [dismiss],
    ),
  )

  const runs = Object.values(active)
  if (runs.length === 0) return null

  return (
    <div
      role="status"
      className="fixed bottom-4 left-1/2 -translate-x-1/2 z-50 max-w-2xl"
    >
      <div className="bg-surface-raised border border-default rounded shadow-lg px-4 py-3">
        <Inline gap="200" align="center" wrap>
          <Inline gap="100" align="center">
            <Badge variant="primary">re-embed</Badge>
            <span className="text-foreground text-sm">
              {runs.length} run{runs.length === 1 ? '' : 's'} in flight
            </span>
          </Inline>
          <Inline gap="100" align="center" wrap>
            {runs.map((r) => (
              <Link
                key={r.id}
                to={`/batch-runs/${r.id}`}
                className="text-foreground text-sm font-medium underline-offset-2 hover:underline"
              >
                #{r.id} {r.namespace}
              </Link>
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
