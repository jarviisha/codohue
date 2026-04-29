interface Props {
  name: string
  status: string
}

const statusColor: Record<string, string> = {
  ok: '#34a853',
  degraded: '#fbbc04',
}

export default function StatusCard({ name, status }: Props) {
  const color = statusColor[status] ?? '#ea4335'
  return (
    <div className="bg-white border border-gray-200 rounded-lg px-5 py-4 flex items-center gap-3 min-w-[180px]">
      <span className="w-3 h-3 rounded-full shrink-0" style={{ background: color }} aria-hidden="true" />
      <div>
        <div className="font-semibold capitalize">{name}</div>
        <div className="text-sm" style={{ color }}>{status}</div>
      </div>
    </div>
  )
}
