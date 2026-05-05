import type { NamespaceStatus } from '../../types'
import { STATUS_META } from './statusMeta'

export default function StatusBadge({ status }: { status: NamespaceStatus }) {
  const m = STATUS_META[status]
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.04em] rounded ${m.wrap} ${m.text}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${m.dot}`} />
      {m.label}
    </span>
  )
}
