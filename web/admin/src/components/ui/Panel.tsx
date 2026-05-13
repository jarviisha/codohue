import type { ReactNode } from 'react'

interface PanelProps {
  title?: ReactNode  // when set, rendered as a mono uppercase section label inside the panel
  actions?: ReactNode
  footer?: ReactNode
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
export default function Panel({ title, actions, footer, children }: PanelProps) {
  const hasHeader = Boolean(title || actions)
  return (
    <section className="bg-surface border border-default rounded-sm">
      {hasHeader ? (
        <div className="flex items-center justify-between gap-2 px-4 pt-3 pb-2">
          {title ? (
            <h2 className="font-mono text-[11px] uppercase tracking-[0.12em] text-muted">
              {title}
            </h2>
          ) : <span />}
          {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
        </div>
      ) : null}
      <div className={hasHeader ? 'px-4 pb-4' : 'p-4'}>{children}</div>
      {footer ? (
        <div className="px-4 py-3 border-t border-default text-sm text-muted">{footer}</div>
      ) : null}
    </section>
  )
}
