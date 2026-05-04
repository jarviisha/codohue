import type { ReactNode } from 'react'

interface EmptyStateProps {
  children: ReactNode
  className?: string
}

export default function EmptyState({ children, className = '' }: EmptyStateProps) {
  return (
    <div className={`p-10 text-center text-sm text-muted border border-dashed border-default rounded-lg ${className}`}>
      {children}
    </div>
  )
}
