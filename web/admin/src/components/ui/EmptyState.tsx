import type { ReactNode } from 'react'

interface EmptyStateProps {
  children: ReactNode
  className?: string
}

export default function EmptyState({ children, className = '' }: EmptyStateProps) {
  return (
    <div className={`rounded-lg border border-dashed border-default p-10 text-center text-sm text-muted ${className}`}>
      {children}
    </div>
  )
}
