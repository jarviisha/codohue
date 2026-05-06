import { useState, useEffect } from 'react'
import NavLink from './NavLink'
import NamespacePicker from './NamespacePicker'
import Icon from './Icon'
import { logout } from '../services/api'
import { navRoutes } from '../routes'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { Button } from './ui'

export default function Sidebar() {
  const [dark, setDark] = useState(() => {
    const savedTheme = localStorage.getItem('theme')
    if (savedTheme === 'dark') return true
    if (savedTheme === 'light') return false
    return document.documentElement.classList.contains('dark')
  })
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
      className="flex w-full shrink-0 flex-col border-b border-default bg-surface px-3 md:fixed md:left-0 md:top-0 md:h-screen md:w-64 md:border-b-0 md:border-r"
    >
      <div className="flex h-14 items-center px-3">
        <div className="min-w-0">
          <span className="block text-base font-semibold leading-tight text-primary">
            Codohue
          </span>
          <span className="block text-[11px] font-medium uppercase tracking-[0.06em] text-muted">
            Admin
          </span>
        </div>
      </div>

      <div className="shrink-0 border-y border-default py-4">
        <p className="m-0 mb-2 px-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
          Namespace
        </p>
        <NamespacePicker />
      </div>

      <div className="flex flex-1 flex-col overflow-y-auto py-4">
        <p className="m-0 mb-2 px-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
          Navigation
        </p>
        <div className="space-y-1">
          {navRoutes.map(route => (
            <NavLink key={route.path} to={`/${route.path}`} icon={route.icon}>{route.label}</NavLink>
          ))}
          {namespace && (
            <NavLink to={`/namespaces/${namespace}`} icon="settings">Settings</NavLink>
          )}
        </div>
      </div>

      <div className="flex items-center justify-between gap-2 border-t border-default py-3">
        <Button
          size="sm"
          variant="ghost"
          onClick={handleLogout}
          className="flex-1 text-left"
        >
          Sign out
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={() => setDark(d => !d)}
          aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {dark ? <Icon name="sun" size={14} /> : <Icon name="moon" size={14} />}
        </Button>
      </div>
    </nav>
  )
}
