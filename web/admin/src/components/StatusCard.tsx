import { Badge } from './ui'

interface Props {
  name: string
  status: string
}

const STATUS: Record<string, { tone: 'success' | 'warning' | 'danger'; frameClass: string }> = {
  ok: {
    tone: 'success',
    frameClass: 'border border-success/30',
  },
  degraded: {
    tone: 'warning',
    frameClass: 'border border-warning/30',
  },
}

const ERROR_STATUS = {
  tone: 'danger' as const,
  frameClass: 'bg-danger-bg border border-danger/25',
}

export default function StatusCard({ name, status }: Props) {
  const s = STATUS[status] ?? ERROR_STATUS

  return (
    <div className={`flex min-w-44 items-center justify-between gap-4 rounded bg-surface px-5 py-4 ${s.frameClass}`}>
      <div>
        <div className="text-sm font-medium text-primary">{name}</div>
      </div>
      <Badge tone={s.tone} dot>{status}</Badge>
    </div>
  )
}
