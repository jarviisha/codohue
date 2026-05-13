import { useState, type ReactNode } from 'react'

interface CollapsibleSectionProps {
  title: ReactNode
  /** Right-aligned widgets shown next to the toggle (e.g. a Badge count). */
  actions?: ReactNode
  /** Initial open state. Defaults to true. */
  defaultOpen?: boolean
  children: ReactNode
}

// Bordered form section with a bolder title than `Section` and a text-based
// expand/collapse toggle (per the no-icons rule). Use for form blocks where
// the visual zone needs to read as "card" and the user should be able to
// shut tuning knobs out of sight.
export default function CollapsibleSection({
  title,
  actions,
  defaultOpen = true,
  children,
}: CollapsibleSectionProps) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <section className="bg-surface border border-default rounded-sm">
      <div className="flex items-center justify-between gap-3 px-4 py-3">
        <h3 className="text-sm font-semibold text-primary flex-1 min-w-0">
          {title}
        </h3>
        <div className="flex items-center gap-3 shrink-0">
          {actions}
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-expanded={open}
            aria-label={open ? 'Collapse section' : 'Expand section'}
            className="font-mono text-xs uppercase tracking-[0.06em] text-muted hover:text-primary"
          >
            {open ? 'collapse' : 'expand'}
          </button>
        </div>
      </div>
      {open ? (
        <div className="px-4 pb-4 pt-3 border-t border-default">{children}</div>
      ) : null}
    </section>
  )
}
