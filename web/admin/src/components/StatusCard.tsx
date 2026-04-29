interface Props {
  name: string
  status: string
}

const colors: Record<string, string> = {
  ok: '#34a853',
  degraded: '#fbbc04',
}

export default function StatusCard({ name, status }: Props) {
  const color = colors[status] ?? '#ea4335'
  return (
    <div style={{ background: '#fff', border: '1px solid #e0e0e0', borderRadius: 8, padding: '1rem 1.25rem', display: 'flex', alignItems: 'center', gap: '0.75rem', minWidth: 180 }}>
      <span style={{ width: 12, height: 12, borderRadius: '50%', background: color, flexShrink: 0 }} aria-hidden="true" />
      <div>
        <div style={{ fontWeight: 600, textTransform: 'capitalize' }}>{name}</div>
        <div style={{ fontSize: '0.85rem', color }}>{status}</div>
      </div>
    </div>
  )
}
