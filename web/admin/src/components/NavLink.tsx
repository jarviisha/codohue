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
        `flex items-center gap-2.5 h-9 px-3 text-sm font-medium rounded no-underline transition-colors duration-150 ${
          isActive
            ? 'bg-accent-subtle text-accent'
            : 'text-secondary hover:bg-surface-raised hover:text-primary'
        }`
      }
    >
      {icon && <Icon name={icon} size={16} className="shrink-0" />}
      <span>{children}</span>
    </RouterNavLink>
  )
}
