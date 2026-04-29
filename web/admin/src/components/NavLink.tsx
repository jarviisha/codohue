import { NavLink as RouterNavLink } from 'react-router-dom'

interface Props {
  to: string
  children: React.ReactNode
}

export default function NavLink({ to, children }: Props) {
  return (
    <RouterNavLink
      to={to}
      style={({ isActive }) => ({
        display: 'block',
        padding: '0.5rem 1rem',
        color: isActive ? '#1a73e8' : '#333',
        textDecoration: 'none',
        borderRadius: 4,
        background: isActive ? '#e8f0fe' : 'transparent',
        fontWeight: isActive ? 600 : 400,
      })}
    >
      {children}
    </RouterNavLink>
  )
}
