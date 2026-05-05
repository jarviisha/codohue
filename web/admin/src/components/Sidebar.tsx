import { useState, useEffect } from 'react'
import NavLink from './NavLink'
import NamespacePicker from './NamespacePicker'
import Icon from './Icon'
import { logout } from '../services/api'
import { navRoutes } from '../routes'
import { useActiveNamespace } from '../context/NamespaceContext'

export default function Sidebar() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains('dark'))
  const { namespace } = useActiveNamespace()

  useEffect(() => {
    if (dark) {
      document.documentElement.classList.add('dark')
      localStorage.setItem('theme', 'dark')
    } else {
      document.documentElement.classList.remove('dark')
      localStorage.setItem('theme', 'light')
    }
  }, [dark])

  async function handleLogout() {
    await logout().catch(() => null)
    window.location.href = '/login'
  }

  return (
    <nav
      aria-label="Main navigation"
      className="w-64 px-3 shrink-0 border-r border-default flex flex-col fixed top-0 left-0 h-screen"
    >
      {/* Brand + dark toggle */}
      <div className="flex items-center justify-between h-14 px-3">
        <span className="text text-2xl text-primary font-semibold underline underline-offset-6 tracking-tight">
          @codohue
        </span>
      </div>

      {/* Namespace selector */}
      <div className="pt-4 pb-3 shrink-0">
        <NamespacePicker />
      </div>

      {/* Nav */}
      <div className="flex flex-col flex-1 py-4 overflow-y-auto">
        <div className="space-y-2">
          {navRoutes.map(route => (
            <NavLink key={route.path} to={`/${route.path}`} icon={route.icon}>{route.label}</NavLink>
          ))}
          {namespace && (
            <NavLink to={`/namespaces/${namespace}`} icon='settings'>Settings</NavLink>
          )}
        </div>
      </div>

      {/* Sign out */}
      <div className="px-3 pb-4 pt-2 border-t border-default flex items-center justify-between">
        <button
          onClick={handleLogout}
          className="flex items-center h-9 px-3 text-sm font-medium text-secondary rounded cursor-pointer hover:bg-surface-raised hover:text-primary transition-colors duration-150"
        >
          Sign out
        </button>
        <button
          onClick={() => setDark(d => !d)}
          className="w-8 h-8 flex items-center justify-center rounded-full text-muted hover:bg-surface-raised hover:text-secondary transition-colors duration-150"
          aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {dark ? <Icon name="sun" size={14} /> : <Icon name="moon" size={14} />}
        </button>
      </div>
    </nav>
  )
}
