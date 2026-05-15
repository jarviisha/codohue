import type { ReactNode } from 'react'

interface MetricTileProps {
  label: ReactNode
  value: ReactNode
  sub?: ReactNode
  subClassName?: string
  valueClassName?: string
  className?: string
}

export default function MetricTile({
  label,
  value,
  sub,
  subClassName = 'text-muted',
  valueClassName = 'text-xl',
  className = '',
}: MetricTileProps) {
  return (
    <div className={`bg-surface border border-default rounded px-3 py-2.5 ${className}`}>
      <p className="m-0 mb-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
        {label}
      </p>
      <p className={`m-0 font-semibold leading-tight text-primary tabular-nums ${valueClassName}`}>
        {value}
      </p>
      {sub && <p className={`m-0 mt-1 text-xs ${subClassName}`}>{sub}</p>}
    </div>
  )
}
