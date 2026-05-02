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
        `block px-3 py-2 text-sm no-underline transition-colors rounded ${
          isActive
            ? 'text-[#533afd] bg-[rgba(83,58,253,0.06)] font-normal'
            : 'text-[#273951] font-normal hover:text-[#533afd] hover:bg-[rgba(83,58,253,0.04)]'
        }`
      }
      style={{ borderRadius: '4px' }}
    >
      {children}
    </RouterNavLink>
  )
}
