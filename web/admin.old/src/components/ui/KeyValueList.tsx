import type { ReactNode } from 'react'

export function KeyValueList({
  children,
  className = '',
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div className={`divide-y divide-default ${className}`}>
      {children}
    </div>
  )
}

export function KeyValueRow({
  label,
  value,
  className = '',
}: {
  label: ReactNode
  value: ReactNode
  className?: string
}) {
  return (
    <div className={`flex items-center justify-between gap-4 py-2.5 ${className}`}>
      <span className="text-sm text-muted">{label}</span>
      <span className="text-sm font-medium text-primary tabular-nums">{value}</span>
    </div>
  )
}
