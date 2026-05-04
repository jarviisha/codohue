interface Props {
  name: string
  status: string
}

const STATUS: Record<string, { dot: string; text: string; wrap: string }> = {
  ok: {
    dot:  'bg-success',
    text: 'text-success',
    wrap: 'bg-success-bg border border-success/30',
  },
  degraded: {
    dot:  'bg-warning',
    text: 'text-warning',
    wrap: 'bg-warning-bg border border-warning/30',
  },
}

const ERROR_STATUS = {
  dot:  'bg-danger',
  text: 'text-danger',
  wrap: 'bg-danger-bg border border-danger/25',
}

export default function StatusCard({ name, status }: Props) {
  const s = STATUS[status] ?? ERROR_STATUS

  return (
    <div className={`flex items-center gap-3 px-5 py-4 min-w-[180px] rounded-lg ${s.wrap}`}>
      <span className={`w-2 h-2 rounded-full shrink-0 ${s.dot}`} aria-hidden="true" />
      <div>
        <div className="text-sm font-medium text-primary">{name}</div>
        <div className={`text-xs tabular-nums ${s.text}`}>{status}</div>
      </div>
    </div>
  )
}
