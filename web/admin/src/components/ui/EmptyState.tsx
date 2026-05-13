import type { ReactNode } from 'react'

interface EmptyStateProps {
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
}

export default function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center text-center py-12 px-5 border border-dashed border-default rounded-sm bg-surface">
      <p className="font-mono text-xs uppercase tracking-[0.04em] text-secondary mb-2">empty</p>
      <h3 className="text-sm font-semibold text-primary mb-1">{title}</h3>
      {description ? <p className="text-sm text-muted leading-5 max-w-sm">{description}</p> : null}
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  )
}
