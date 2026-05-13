import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: ReactNode
  /** Mono meta strip rendered below the title row. Holds counters,
   *  namespace name, inline StatusToken, etc. The row is always
   *  reserved — passing nothing still keeps the header height
   *  constant across pages. */
  meta?: ReactNode
  /** Right-aligned page-level actions. Vertically centered with the title. */
  actions?: ReactNode
}

// Top-of-page header. Two-row layout:
//   row 1 — title (text-xl semibold) + actions (right-aligned)
//   row 2 — meta strip (mono, text-sm, muted) full-width below
// Both rows have reserved heights (min-h-9 + min-h-5) so navigating
// between pages does not cause the header — and therefore all content
// below it — to bounce vertically when meta is absent or loading.
// A subtle bottom border separates the header from page content.
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
      <div className="mt-1 text-sm font-mono text-muted min-h-5">
        {meta}
      </div>
    </header>
  )
}
