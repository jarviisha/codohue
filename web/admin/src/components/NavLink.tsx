import { NavLink as RouterNavLink } from 'react-router-dom'
import Icon from './Icon'
import type { IconName } from './icons'

interface Props {
  to: string
  icon?: IconName
  children: React.ReactNode
}

export default function NavLink({ to, icon, children }: Props) {
  return (
    <RouterNavLink
      to={to}
      className={({ isActive }) =>
        `flex h-9 items-center gap-2.5 rounded-lg border px-3 text-sm font-medium no-underline transition-colors duration-150 focus-visible:outline-none focus-visible:shadow-focus ${
          isActive
            ? 'border-accent/20 bg-accent-subtle text-accent'
            : 'border-transparent text-secondary hover:bg-surface-raised hover:text-primary'
        }`
      }
    >
      {icon && <Icon name={icon} size={16} className="shrink-0" />}
      <span>{children}</span>
    </RouterNavLink>
  )
}
