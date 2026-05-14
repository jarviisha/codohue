import type { ReactNode } from 'react'
import { Link, useMatch, useResolvedPath } from 'react-router-dom'

interface TabsProps {
  ariaLabel?: string
  children: ReactNode
}

// Section tab bar that sits flush under PageHeader. Use TabLink children when
// the tabs correspond to nested routes (preferred — keeps URL canonical).
export function Tabs({ ariaLabel = 'Sections', children }: TabsProps) {
  return (
    <div className="z-10 -mx-6 bg-base px-6 py-4">
      <div role="tablist" aria-label={ariaLabel} className="flex gap-4">
        {children}
      </div>
    </div>
  )
}

interface TabLinkProps {
  to: string
  end?: boolean
  children: ReactNode
}

export function TabLink({ to, end, children }: TabLinkProps) {
  const resolved = useResolvedPath(to)
  const active = Boolean(useMatch({ path: resolved.pathname, end }))
  return (
    <Link
      to={to}
      role="tab"
      aria-selected={active}
      className={[
        'h-8 px-3 rounded-sm font-mono text-xs uppercase tracking-[0.04em] inline-flex items-center',
        active
          ? 'bg-accent-subtle text-accent'
          : 'text-muted hover:bg-surface-raised hover:text-primary',
      ].join(' ')}
    >
      {children}
    </Link>
  )
}
