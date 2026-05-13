import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: ReactNode
  /** Mono meta strip rendered below the title row. Holds counters,
   *  namespace name, inline StatusToken, etc. */
  meta?: ReactNode
  /** Right-aligned page-level actions. Vertically centered with the title. */
  actions?: ReactNode
}

// Top-of-page header. Two-row layout when meta is present:
//   row 1 — title (text-xl semibold) + actions (right-aligned, centered)
//   row 2 — meta strip (mono, text-sm, muted) full-width below
// A subtle bottom border separates the header from page content; the rest
// of the page sits below via PageShell's gap-4 between children.
export default function PageHeader({ title, meta, actions }: PageHeaderProps) {
  return (
    <header className="border-b border-default pb-3">
      <div className="flex items-center justify-between gap-4 min-h-9">
        <h1 className="text-xl font-semibold text-primary leading-tight min-w-0">
          {title}
        </h1>
        {actions ? (
          <div className="flex items-center gap-2 shrink-0">{actions}</div>
        ) : null}
      </div>
      {meta ? (
        <div className="mt-1 text-sm font-mono text-muted">{meta}</div>
      ) : null}
    </header>
  )
}
