import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useToast } from '@jarviisha/davinci-react-ui'
import { useServerStream } from '@/services/stream'

/**
 * OpsToastBridge subscribes once at the AppShell level to the global ops
 * stream and raises toast notifications for events that an operator needs
 * to see no matter which page they're on.
 *
 * Surfaced events:
 *   - batch_run.completed (success=false)  → danger toast w/ "Open run" action
 *   - batch_run.cancelled                  → warning toast w/ "Open run" action
 *   - catalog.dead_letter_grew             → warning toast w/ "Open catalog" action
 *
 * Successful completions and `started` events are intentionally silent — the
 * fleet page already shows them ambiently and a flood of green toasts during
 * the cron tick is noise, not signal.
 */
export default function OpsToastBridge() {
  const toast = useToast()
  const navigate = useNavigate()

  useServerStream(
    '/api/admin/v1/stream',
    useMemo(
      () => ({
        completed: (data: unknown) => {
          const d = data as {
            id?: number
            namespace?: string
            success?: boolean
            kind?: string
          }
          if (d.success !== false) return
          toast.danger(
            d.kind === 'reembed' ? 'Re-embed run failed' : 'Batch run failed',
            {
              description: `${d.namespace ?? '?'} · run #${d.id ?? '?'}`,
              action:
                d.id != null
                  ? {
                      label: 'Open run',
                      onClick: () => navigate(`/batch-runs/${d.id}`),
                    }
                  : undefined,
            },
          )
        },
        cancelled: (data: unknown) => {
          const d = data as { id?: number; namespace?: string }
          toast.warning('Batch run cancelled', {
            description: `${d.namespace ?? '?'} · run #${d.id ?? '?'}`,
            action:
              d.id != null
                ? {
                    label: 'Open run',
                    onClick: () => navigate(`/batch-runs/${d.id}`),
                  }
                : undefined,
          })
        },
        dead_letter_grew: (data: unknown) => {
          const d = data as { namespace?: string; new_count?: number; delta?: number }
          if (!d.namespace || !d.delta) return
          toast.warning(`Dead-letter grew by ${d.delta}`, {
            description: `${d.namespace} now has ${d.new_count ?? 0} dead-letter item(s).`,
            action: {
              label: 'Open catalog',
              onClick: () => navigate(`/ns/${encodeURIComponent(d.namespace!)}/catalog`),
            },
          })
        },
      }),
      [toast, navigate],
    ),
  )

  return null
}
