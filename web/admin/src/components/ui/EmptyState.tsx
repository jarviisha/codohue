import type { ReactNode } from 'react'

interface EmptyStateProps {
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
}

export default function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center text-center py-12 px-4 border border-dashed border-default rounded-sm bg-transparent">
      <p className="font-mono text-[11px] uppercase tracking-[0.12em] text-muted mb-2">empty</p>
      <h3 className="text-sm font-semibold text-primary mb-1">{title}</h3>
      {description ? <p className="text-sm text-muted max-w-sm">{description}</p> : null}
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  )
}
