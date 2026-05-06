import type { NamespaceStatus } from '../../types'
import { STATUS_META } from './statusMeta'
import { Badge } from '../../components/ui'

export default function StatusBadge({ status }: { status: NamespaceStatus }) {
  const m = STATUS_META[status]
  return (
    <Badge tone={m.tone} dot>
      {m.label}
    </Badge>
  )
}
