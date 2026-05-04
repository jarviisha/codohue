import { useState, useEffect } from 'react'
import { Outlet } from 'react-router-dom'
import NavLink from './NavLink'
import { logout } from '../services/api'
import { navRoutesForSection, navSections } from '../routes'

function SunIcon() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="4"/>
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/>
    </svg>
  )
}

function MoonIcon() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
    </svg>
  )
}

export default function Layout() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains('dark'))

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
    <div className="flex min-h-screen bg-base">
      <nav
        aria-label="Main navigation"
        className="w-60 shrink-0 bg-subtle border-r border-default flex flex-col fixed top-0 left-0 h-screen"
      >
        {/* Brand + dark toggle */}
        <div className="flex items-center justify-between px-6 h-14 border-b border-default shrink-0">
          <span className="text-[15px] font-semibold text-primary tracking-tight">
            Codohue
            <span className="text-accent font-medium ml-1">Admin</span>
          </span>
          <button
            onClick={() => setDark(d => !d)}
            className="w-7 h-7 flex items-center justify-center rounded-md text-muted hover:bg-surface-raised hover:text-secondary transition-colors duration-150"
            aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
          >
            {dark ? <SunIcon /> : <MoonIcon />}
          </button>
        </div>

        <div className="flex flex-col flex-1 px-3 py-4 gap-0.5 overflow-y-auto">
          {navSections.map((section, index) => (
            <div key={section}>
              <p className={`text-[11px] font-semibold uppercase tracking-[0.06em] text-muted px-3 pb-1.5 ${index === 0 ? 'pt-1' : 'pt-4'}`}>
                {section}
              </p>
              {navRoutesForSection(section).map(route => (
                <NavLink key={route.path} to={`/${route.path}`}>{route.label}</NavLink>
              ))}
            </div>
          ))}
        </div>

        {/* Sign out */}
        <div className="px-3 pb-4 shrink-0">
          <button
            onClick={handleLogout}
            className="w-full h-9 px-3 text-sm font-medium text-secondary bg-transparent border border-default rounded-md cursor-pointer hover:bg-surface-raised hover:text-primary hover:border-strong transition-colors duration-150"
          >
            Sign out
          </button>
        </div>
      </nav>

      <main className="flex-1 bg-base min-h-screen ml-60">
        <div className="px-8 py-8">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
