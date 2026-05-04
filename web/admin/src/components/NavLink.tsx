import { NavLink as RouterNavLink } from 'react-router-dom'

interface Props {
  to: string
  children: React.ReactNode
}

export default function NavLink({ to, children }: Props) {
  return (
    <RouterNavLink
      to={to}
      className={({ isActive }) =>
        `flex items-center h-9 px-3 text-sm font-medium rounded-md no-underline transition-colors duration-150 ${
          isActive
            ? 'bg-accent-subtle text-accent'
            : 'text-secondary hover:bg-surface-raised hover:text-primary'
        }`
      }
    >
      {children}
    </RouterNavLink>
  )
}
