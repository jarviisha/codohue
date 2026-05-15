import { Outlet, useLocation } from 'react-router-dom'
import Sidebar from './Sidebar'
import ThemeToggle from './ThemeToggle'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { EmptyState } from './ui'
import { appLayoutClasses } from './appLayoutClasses'

const ROUTES_WITHOUT_NS = ['/namespaces', '/health']

export default function Layout() {
  const { namespace } = useActiveNamespace()
  const location = useLocation()

  const needsNs = !ROUTES_WITHOUT_NS.some(p => location.pathname.startsWith(p))

  return (
    <div className="min-h-screen bg-base">
      <Sidebar />
      <ThemeToggle />
      <main className={appLayoutClasses.main}>
        <div className={appLayoutClasses.content}>
          {needsNs && !namespace ? (
            <EmptyState className="mt-16">
              Select a namespace from the sidebar to continue.
            </EmptyState>
          ) : (
            <Outlet />
          )}
        </div>
      </main>
    </div>
  )
}
