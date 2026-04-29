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
        `block mx-2 px-3 py-2 rounded text-sm no-underline transition-colors ${
          isActive
            ? 'text-blue-600 bg-blue-50 font-semibold'
            : 'text-gray-700 font-normal hover:bg-gray-100'
        }`
      }
    >
      {children}
    </RouterNavLink>
  )
}
