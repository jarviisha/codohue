import { Outlet, useLocation } from 'react-router-dom'
import Sidebar from './Sidebar'
import Breadcrumbs from './Breadcrumbs'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { EmptyState } from './ui'

const ROUTES_WITHOUT_NS = ['/namespaces', '/health']

export default function Layout() {
  const { namespace } = useActiveNamespace()
  const location = useLocation()

  const needsNs = !ROUTES_WITHOUT_NS.some(p => location.pathname.startsWith(p))

  return (
    <div className="flex min-h-screen bg-base">
      <Sidebar />
      <main className="ml-64 min-h-screen flex-1 bg-base">
        <div className="max-w-7xl mx-auto px-8 py-8">
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
