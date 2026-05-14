import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: ReactNode
  /** Deprecated: page metadata now belongs in the page body, not the fixed header. */
  meta?: ReactNode
  /** Right-aligned page-level commands. Prefer secondary/ghost actions. */
  actions?: ReactNode
  /** Deprecated: labels are no longer rendered in the compact fixed header. */
  label?: ReactNode
}

// Compact console-style page header. It stays pinned to the top of the
// scrollable page body; page-specific status/details belong in panels below.
export default function PageHeader({
  title,
  actions,
}: PageHeaderProps) {
  return (
    <header className="sticky top-0 z-20 -mx-6 px-6 py-6 bg-base border-b border-default">
      <div className="flex items-center justify-between gap-4">
        <div className="min-w-0">
          <h1 className="font-mono text-xl text-primary leading-6 truncate lowercase">
            {title}
          </h1>
        </div>
        {actions ? (
          <div className="page-header-actions flex items-center gap-2 shrink-0">
            {actions}
          </div>
        ) : null}
      </div>
    </header>
  )
}
