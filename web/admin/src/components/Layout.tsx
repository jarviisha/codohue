import { Outlet, useLocation } from 'react-router-dom'
import Sidebar from './Sidebar'
import Breadcrumbs from './Breadcrumbs'
import { useActiveNamespace } from '../context/NamespaceContext'

const ROUTES_WITHOUT_NS = ['/namespaces', '/health']

export default function Layout() {
  const { namespace } = useActiveNamespace()
  const location = useLocation()

  const needsNs = !ROUTES_WITHOUT_NS.some(p => location.pathname.startsWith(p))

  return (
    <div className="flex min-h-screen bg-base">
      <Sidebar />
      <main className="flex-1 bg-base min-h-screen ml-64">
        <div className="max-w-7xl mx-auto px-8 py-8">
          {needsNs && !namespace ? (
            <div className="flex flex-col items-center justify-center py-24 gap-3 text-center">
              <p className="text-sm font-semibold text-primary m-0">No namespace selected</p>
              <p className="text-sm text-muted m-0">
                Select a namespace from the sidebar to continue.
              </p>
            </div>
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
