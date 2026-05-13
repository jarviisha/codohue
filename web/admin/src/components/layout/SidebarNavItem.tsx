import type { ReactNode } from 'react'
import { NavLink } from 'react-router-dom'

interface SidebarNavItemProps {
  to: string
  end?: boolean
  trailing?: ReactNode // optional right-aligned widget (e.g. inline StatusToken)
  children: ReactNode
}

// Sidebar nav row. Active state is carried by bg-accent-subtle + text-accent.
// No leading-glyph column — icons are deferred until the user supplies an
// icon set.
export default function SidebarNavItem({ to, end, trailing, children }: SidebarNavItemProps) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        [
          'flex items-center h-8 px-3 text-sm rounded-sm transition-colors duration-100',
          isActive
            ? 'bg-accent-subtle text-accent'
            : 'text-secondary hover:bg-surface-raised hover:text-primary',
        ].join(' ')
      }
    >
      <span className="flex-1">{children}</span>
      {trailing ? <span className="ml-2">{trailing}</span> : null}
    </NavLink>
  )
}
