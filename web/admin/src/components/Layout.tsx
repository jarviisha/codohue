import { Outlet } from 'react-router-dom'
import NavLink from './NavLink'
import { logout } from '../services/api'

export default function Layout() {
  async function handleLogout() {
    await logout().catch(() => null)
    window.location.href = '/login'
  }

  return (
    <div className="flex min-h-screen">
      <nav
        aria-label="Main navigation"
        className="w-[200px] shrink-0 bg-white border-r border-gray-200 py-4 flex flex-col"
      >
        <div className="px-4 pb-4 font-bold text-lg text-blue-600">
          Codohue Admin
        </div>
        <NavLink to="/health">System Health</NavLink>
        <NavLink to="/namespaces">Namespaces</NavLink>
        <NavLink to="/debug">Recommend Debug</NavLink>
        <NavLink to="/batch-runs">Batch Runs</NavLink>
        <NavLink to="/trending">Trending</NavLink>
        <div className="flex-1" />
        <div className="px-2">
          <button
            onClick={handleLogout}
            className="w-full py-2 bg-transparent border border-gray-300 rounded cursor-pointer text-gray-500 text-sm hover:bg-gray-50"
          >
            Sign out
          </button>
        </div>
      </nav>
      <main className="flex-1 p-6 bg-gray-50 overflow-y-auto">
        <Outlet />
      </main>
    </div>
  )
}
