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

export function TriggerBadge({ source }: { source: string }) {
  switch (source) {
    case 'manual':
    case 'admin':
      return <Badge tone="warning">{source}</Badge>
    case 'admin_reembed':
      return <Badge tone="accent">re-embed</Badge>
    default:
      return <Badge>{source || 'cron'}</Badge>
  }
}
