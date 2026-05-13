import type { ReactNode } from 'react'

interface MetricTileProps {
  label: ReactNode
  value: ReactNode
  hint?: ReactNode // small mono text below the value
}

// Uniform metric tile. No hero variant — operators want uniform scan (DESIGN.md §16).
export default function MetricTile({ label, value, hint }: MetricTileProps) {
  return (
    <div className="bg-surface border border-default rounded-sm p-4 flex flex-col gap-1">
      <span className="font-mono text-[11px] uppercase tracking-[0.12em] text-muted">{label}</span>
      <span className="font-mono tabular-nums text-2xl text-primary leading-tight">{value}</span>
      {hint ? <span className="font-mono text-xs text-muted">{hint}</span> : null}
    </div>
  )
}
