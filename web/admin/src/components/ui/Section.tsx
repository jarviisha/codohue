import type { ReactNode } from 'react'

interface SectionProps {
  title?: ReactNode
  actions?: ReactNode
  /** Vertical gap inside the section. Defaults to `gap-4`. */
  gap?: 'gap-2' | 'gap-3' | 'gap-4' | 'gap-5'
  children: ReactNode
}

// Borderless content group with an optional mono-uppercase title label and
// optional right-aligned actions. Use this when you want to group related
// content under a heading without surrounding it with a bordered card —
// stacking many `Panel`s leads to nested rectangles and a cluttered feel
// (e.g. inside a multi-section form). For true card semantics use `Panel`.
export default function Section({
  title,
  actions,
  gap = 'gap-4',
  children,
}: SectionProps) {
  const hasHeader = Boolean(title || actions)
  return (
    <section className={`flex flex-col ${gap}`}>
      {hasHeader ? (
        <header className="flex items-center justify-between gap-2">
          {title ? (
            <h2 className="font-mono text-xs uppercase tracking-[0.04em] text-secondary">
              {title}
            </h2>
          ) : (
            <span />
          )}
          {actions ? (
            <div className="flex items-center gap-2">{actions}</div>
          ) : null}
        </header>
      ) : null}
      <div>{children}</div>
    </section>
  )
}
