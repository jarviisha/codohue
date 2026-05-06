import type { NamespaceHealth, NamespaceStatus } from '../../types'
import { STATUS_META } from './statusMeta'
import { Badge } from '../../components/ui'

export default function SummaryBar({ namespaces }: { namespaces: NamespaceHealth[] }) {
  const counts = { active: 0, idle: 0, degraded: 0, cold: 0 }
  for (const n of namespaces) counts[n.status]++

  return (
    <div className="flex flex-wrap gap-3">
      {(Object.entries(counts) as [NamespaceStatus, number][]).map(([status, count]) => {
        const m = STATUS_META[status]
        return (
          <Badge key={status} tone={m.tone} size="md" dot>
            <span className="tabular-nums font-semibold">{count}</span>
            <span className="text-xs">{m.label}</span>
          </Badge>
        )
      })}
    </div>
  )
}
