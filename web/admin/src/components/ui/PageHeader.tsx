import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: ReactNode
  meta?: ReactNode  // small mono text under the title (counts, last updated, etc.)
  actions?: ReactNode // right-aligned buttons
}

export default function PageHeader({ title, meta, actions }: PageHeaderProps) {
  return (
    <header className="flex items-start justify-between gap-4">
      <div className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold text-primary leading-tight">{title}</h1>
        {meta ? <div className="text-xs text-muted font-mono">{meta}</div> : null}
      </div>
      {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
    </header>
  )
}
