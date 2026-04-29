import { Outlet } from 'react-router-dom'
import NavLink from './NavLink'
import { logout } from '../services/api'

export default function Layout() {
  async function handleLogout() {
    await logout().catch(() => null)
    window.location.href = '/login'
  }

  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      <nav
        aria-label="Main navigation"
        style={{ width: 200, background: '#fff', borderRight: '1px solid #e0e0e0', padding: '1rem 0', display: 'flex', flexDirection: 'column' }}
      >
        <div style={{ padding: '0 1rem 1rem', fontWeight: 700, fontSize: '1.1rem', color: '#1a73e8' }}>
          Codohue Admin
        </div>
        <NavLink to="/health">System Health</NavLink>
        <NavLink to="/namespaces">Namespaces</NavLink>
        <NavLink to="/debug">Recommend Debug</NavLink>
        <NavLink to="/batch-runs">Batch Runs</NavLink>
        <NavLink to="/trending">Trending</NavLink>
        <div style={{ flex: 1 }} />
        <div style={{ padding: '0 0.5rem' }}>
          <button
            onClick={handleLogout}
            style={{ width: '100%', padding: '0.5rem', background: 'none', border: '1px solid #ccc', borderRadius: 4, cursor: 'pointer', color: '#666' }}
          >
            Sign out
          </button>
        </div>
      </nav>
      <main style={{ flex: 1, padding: '1.5rem', background: '#f8f9fa', overflowY: 'auto' }}>
        <Outlet />
      </main>
    </div>
  )
}
