interface Props {
  name: string
  status: string
}

const STATUS: Record<string, { dot: string; text: string; bg: string; border: string }> = {
  ok: {
    dot: '#15be53',
    text: '#108c3d',
    bg: 'rgba(21,190,83,0.07)',
    border: 'rgba(21,190,83,0.25)',
  },
  degraded: {
    dot: '#f59e0b',
    text: '#92400e',
    bg: 'rgba(245,158,11,0.07)',
    border: 'rgba(245,158,11,0.25)',
  },
}

export default function StatusCard({ name, status }: Props) {
  const s = STATUS[status] ?? {
    dot: '#ea2261',
    text: '#ea2261',
    bg: 'rgba(234,34,97,0.06)',
    border: 'rgba(234,34,97,0.2)',
  }

  return (
    <div
      className="flex items-center gap-3 px-5 py-4 min-w-[180px]"
      style={{
        background: s.bg,
        border: `1px solid ${s.border}`,
        borderRadius: '5px',
      }}
    >
      <span
        className="w-2 h-2 rounded-full shrink-0"
        style={{ background: s.dot }}
        aria-hidden="true"
      />
      <div>
        <div
          className="text-sm font-normal"
          style={{ color: '#061b31' }}
        >
          {name}
        </div>
        <div
          className="text-xs font-normal tabular-nums"
          style={{ color: s.text }}
        >
          {status}
        </div>
      </div>
    </div>
  )
}
