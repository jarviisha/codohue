import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Stack, Text, useToast } from '@astryxdesign/core'
import { useServerStream } from '@/services/stream'
import type { ReactNode } from 'react'

/**
 * OpsToastBridge subscribes once at the AppShell level to the global ops
 * stream and raises toast notifications for events that an operator needs
 * to see no matter which page they're on.
 *
 * Surfaced events:
 *   - batch_run.completed (success=false)  → error toast w/ "Open run" action
 *   - batch_run.cancelled                  → info toast w/ "Open run" action
 *   - catalog.dead_letter_grew             → info toast w/ "Open catalog" action
 *
 * Astryx toasts are only info or error, so the two warning-grade events land on
 * info; the failure is the only one that should stick until acknowledged.
 *
 * Successful completions and `started` events are intentionally silent — the
 * fleet page already shows them ambiently and a flood of green toasts during
 * the cron tick is noise, not signal.
 */
export default function OpsToastBridge() {
  const showToast = useToast()
  const navigate = useNavigate()

  useServerStream(
    '/api/admin/v1/stream',
    useMemo(() => {
      const openRun = (id: number) => (
        <Button
          size="sm"
          variant="ghost"
          label="Open run"
          onClick={() => navigate(`/batch-runs/${id}`)}
        />
      )
      return {
        completed: (data: unknown) => {
          const d = data as {
            id?: number
            namespace?: string
            success?: boolean
            kind?: string
          }
          if (d.success !== false) return
          showToast({
            type: 'error',
            body: toastBody(
              d.kind === 'reembed' ? 'Re-embed run failed' : 'Batch run failed',
              `run #${d.id ?? '?'} in ${d.namespace ?? '?'}`,
            ),
            endContent: d.id != null ? openRun(d.id) : undefined,
          })
        },
        cancelled: (data: unknown) => {
          const d = data as { id?: number; namespace?: string }
          showToast({
            body: toastBody(
              'Batch run cancelled',
              `run #${d.id ?? '?'} in ${d.namespace ?? '?'}`,
            ),
            endContent: d.id != null ? openRun(d.id) : undefined,
          })
        },
        dead_letter_grew: (data: unknown) => {
          const d = data as { namespace?: string; new_count?: number; delta?: number }
          if (!d.namespace || !d.delta) return
          showToast({
            body: toastBody(
              `Dead-letter grew by ${d.delta}`,
              `${d.namespace} now has ${d.new_count ?? 0} dead-letter item(s).`,
            ),
            endContent: (
              <Button
                size="sm"
                variant="ghost"
                label="Open catalog"
                onClick={() => navigate(`/ns/${encodeURIComponent(d.namespace!)}/catalog`)}
              />
            ),
          })
        },
      }
    }, [showToast, navigate]),
  )

  return null
}

// Astryx toasts take a single body node, so the old title + description pair
// is composed here rather than passed as separate props.
function toastBody(title: string, description: string): ReactNode {
  return (
    <Stack gap={0.5}>
      <Text weight="semibold">{title}</Text>
      <Text type="supporting">{description}</Text>
    </Stack>
  )
}
