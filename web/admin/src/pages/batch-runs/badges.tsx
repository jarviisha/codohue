import { Badge } from '../../components/ui'
import type { BatchRunLog } from '../../types'

export function RunStatus({ run }: { run: BatchRunLog }) {
  if (run.success) {
    return (
      <Badge tone="success" dot>
        OK
      </Badge>
    )
  }
  if (run.completed_at) {
    return (
      <Badge tone="danger" dot>
        Failed
      </Badge>
    )
  }
  return (
    <Badge tone="accent" dot>
      Running
    </Badge>
  )
}

export function TriggerBadge({ source }: { source: 'cron' | 'manual' }) {
  if (source === 'manual') {
    return (
      <Badge tone="warning">
        manual
      </Badge>
    )
  }
  return (
    <Badge>
      cron
    </Badge>
  )
}
