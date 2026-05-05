import type { NamespaceHealth, NamespaceStatus } from '../../types'
import { STATUS_META } from './statusMeta'

export default function SummaryBar({ namespaces }: { namespaces: NamespaceHealth[] }) {
  const counts = { active: 0, idle: 0, degraded: 0, cold: 0 }
  for (const n of namespaces) counts[n.status]++

  return (
    <div className="flex gap-3 mb-6 flex-wrap">
      {(Object.entries(counts) as [NamespaceStatus, number][]).map(([status, count]) => {
        const m = STATUS_META[status]
        return (
          <div
            key={status}
            className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded ${m.wrap} ${m.text}`}
          >
            <span className={`w-2 h-2 rounded ${m.dot}`} />
            <span className="tabular-nums font-semibold">{count}</span>
            <span className="text-xs">{m.label}</span>
          </div>
        )
      })}
    </div>
  )
}
