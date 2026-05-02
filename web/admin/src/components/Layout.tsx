import { Outlet } from 'react-router-dom'
import NavLink from './NavLink'
import { logout } from '../services/api'

export default function Layout() {
  async function handleLogout() {
    await logout().catch(() => null)
    window.location.href = '/login'
  }

  return (
    <div className="flex bg-white">
      <nav
        aria-label="Main navigation"
        className="w-[220px] shrink-0 bg-white border-r border-[#e5edf5] flex flex-col py-6 fixed top-0 left-0 h-screen"
      >
        <div className="px-6 pb-6">
          <span
            className="text-[#061b31] font-light tracking-tight"
            style={{ fontSize: '15px', letterSpacing: '-0.2px' }}
          >
            Codohue
          </span>
          <span
            className="ml-1.5 text-[#533afd] font-light"
            style={{ fontSize: '15px' }}
          >
            Admin
          </span>
        </div>

        <div className="flex flex-col gap-0.5 px-3 flex-1">
          <NavLink to="/health">System Health</NavLink>
          <NavLink to="/namespaces">Namespaces</NavLink>
          <NavLink to="/batch-runs">Batch Runs</NavLink>
          <NavLink to="/trending">Trending</NavLink>
          <NavLink to="/debug">Recommend Debug</NavLink>
        </div>

        <div className="px-3 pt-4 border-t border-[#e5edf5] mt-4">
          <button
            onClick={handleLogout}
            className="w-full px-4 py-2 text-[#64748d] text-sm font-normal bg-transparent border border-[#e5edf5] rounded cursor-pointer hover:border-[#b9b9f9] hover:text-[#533afd] transition-colors"
            style={{ borderRadius: '4px' }}
          >
            Sign out
          </button>
        </div>
      </nav>

      <main className="flex-1 bg-white min-h-screen" style={{ marginLeft: '220px' }}>
        <div className="max-w-5xl mx-auto px-8 py-8">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
