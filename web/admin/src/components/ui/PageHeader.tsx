import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: ReactNode
  /** Mono meta strip rendered directly under the title in the same
   *  left column. Always reserves a row so the header height stays
   *  constant across pages. */
  meta?: ReactNode
  /** Right-aligned page-level actions. Baseline-aligned to the title. */
  actions?: ReactNode
}

// title + meta stacked in the left column as one block; actions sit on the
// right, baseline-aligned to the title text. A subtle bottom border separates
// the header from page content.
export default function PageHeader({ title, meta, actions }: PageHeaderProps) {
  return (
    <header className="border-b border-default pb-3 -mx-6 px-6">
      <div className="flex items-center justify-between gap-4">
        <div className="flex flex-col gap-1 min-w-0">
          <h1 className="text-xl font-semibold text-primary leading-tight">
            {title}
          </h1>
          <div className="text-sm font-mono text-muted min-h-5">{meta}</div>
        </div>
        {actions ? (
          <div className="flex items-center gap-2 shrink-0">{actions}</div>
        ) : null}
      </div>
    </header>
  )
}
