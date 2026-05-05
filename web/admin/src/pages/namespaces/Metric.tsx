export default function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="text-center">
      <p className="text-xs text-muted mb-0.5 m-0">{label}</p>
      <p className="text-sm font-medium text-primary truncate m-0 tabular-nums">{value}</p>
    </div>
  )
}
