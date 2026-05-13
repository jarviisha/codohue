import { Outlet, useLocation } from 'react-router-dom'
import Sidebar from './Sidebar'
import Breadcrumbs from './Breadcrumbs'
import ThemeToggle from './ThemeToggle'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { EmptyState } from './ui'

const ROUTES_WITHOUT_NS = ['/namespaces', '/health']

export default function Layout() {
  const { namespace } = useActiveNamespace()
  const location = useLocation()

  const needsNs = !ROUTES_WITHOUT_NS.some(p => location.pathname.startsWith(p))

  return (
    <div className="min-h-screen bg-base">
      <Sidebar />
      <ThemeToggle />
      <main className="min-h-screen bg-base md:ml-56">
        <div className="mx-auto max-w-7xl px-4 py-4 sm:px-5 md:px-6 md:py-6">
          {needsNs && !namespace ? (
            <EmptyState className="mt-16">
              Select a namespace from the sidebar to continue.
            </EmptyState>
          ) : (
            <>
              <Breadcrumbs />
              <Outlet />
            </>
          )}
        </div>
      </main>
    </div>
  )
}
