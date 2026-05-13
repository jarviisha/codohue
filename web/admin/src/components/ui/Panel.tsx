import type { ReactNode } from 'react'

interface PanelProps {
  title?: ReactNode  // when set, rendered as a mono uppercase section label inside the panel
  actions?: ReactNode
  footer?: ReactNode
  children: ReactNode
}

// Bordered surface. No shadow, no nested panels (DESIGN.md §5).
export default function Panel({ title, actions, footer, children }: PanelProps) {
  return (
    <section className="bg-surface border border-default rounded-sm">
      {(title || actions) && (
        <div className="flex items-center justify-between gap-2 px-4 py-3 border-b border-default">
          {title ? (
            <h2 className="font-mono text-[11px] uppercase tracking-[0.12em] text-muted">
              {title}
            </h2>
          ) : <span />}
          {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
        </div>
      )}
      <div className="p-4">{children}</div>
      {footer ? (
        <div className="px-4 py-3 border-t border-default text-sm text-muted">{footer}</div>
      ) : null}
    </section>
  )
}
