import type { ReactNode } from 'react'

interface PanelProps {
  title?: ReactNode  // when set, rendered as a mono uppercase section label inside the panel
  actions?: ReactNode
  footer?: ReactNode
  /**
   * Stale-while-refetch hint. When true, a small pulsing accent dot renders
   * next to the title so background refetches (interval polls, mutation
   * invalidations) are visible. Pages typically pass
   * `busy={query.isFetching && !query.isLoading}`.
   */
  busy?: boolean
  children: ReactNode
}

// Bordered surface. No shadow, no nested panels (DESIGN.md §5).
//
// Title + actions sit flush above the content — no internal divider — to
// keep the panel from feeling like nested rectangles. The mono uppercase
// title is already visually distinct.
//
// For non-card grouping (e.g. form sections), prefer the borderless
// `Section` primitive instead of stacking many Panels.
export default function Panel({ title, actions, footer, busy = false, children }: PanelProps) {
  const hasHeader = Boolean(title || actions || busy)
  return (
    <section className="bg-surface border border-default rounded-sm">
      {hasHeader ? (
        <div className="flex items-center justify-between gap-3 px-5 pt-4 pb-3">
          <div className="flex items-center gap-2 min-w-0">
            {title ? (
              <h2 className="font-mono text-xs uppercase tracking-[0.04em] text-secondary">
                {title}
              </h2>
            ) : null}
            {busy ? (
              <span
                aria-hidden
                title="Refreshing"
                className="inline-block h-1.5 w-1.5 rounded-full bg-accent animate-pulse-run"
              />
            ) : null}
          </div>
          {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
        </div>
      ) : null}
      <div className={hasHeader ? 'px-5 pb-5' : 'p-5'}>{children}</div>
      {footer ? (
        <div className="px-5 py-3 border-t border-default text-sm text-muted leading-5">{footer}</div>
      ) : null}
    </section>
  )
}
