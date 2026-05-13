import type { ReactNode } from 'react'
import type { IconName } from './components/icons'
import CatalogItemsPage from './pages/CatalogItemsPage'
import HealthPage from './pages/HealthPage'
import NamespaceDetailPage from './pages/NamespaceDetailPage'
import NamespaceOverviewPage from './pages/NamespaceOverviewPage'
import RecommendDebugPage from './pages/RecommendDebugPage'
import BatchRunsPage from './pages/BatchRunsPage'
import TrendingPage from './pages/TrendingPage'
import EventsPage from './pages/EventsPage'
import NamespacesPage from './pages/NamespacesPage'

export interface AdminRoute {
  path: string
  label: string
  element: ReactNode
  nav?: boolean
  scope?: 'global' | 'namespace'
  icon?: IconName
}

export const adminRoutes: AdminRoute[] = [
  {
    path: 'namespaces/:ns/overview',
    label: 'Overview',
    element: <NamespaceOverviewPage />,
    nav: true,
    scope: 'namespace',
    icon: 'home',
  },
  {
    path: 'namespaces/:ns/batch-runs',
    label: 'Batch Runs',
    element: <BatchRunsPage />,
    nav: true,
    scope: 'namespace',
    icon: 'clock-hour-11',
  },
  {
    path: 'namespaces/:ns/events',
    label: 'Events',
    element: <EventsPage />,
    nav: true,
    scope: 'namespace',
    icon: 'bell',
  },
  {
    path: 'namespaces/:ns/trending',
    label: 'Trending',
    element: <TrendingPage />,
    nav: true,
    scope: 'namespace',
    icon: 'arrow-big-up-line',
  },
  {
    path: 'namespaces/:ns/debug',
    label: 'Recommend Debug',
    element: <RecommendDebugPage />,
    nav: true,
    scope: 'namespace',
    icon: 'search',
  },
  {
    path: 'health',
    label: 'System Health',
    element: <HealthPage />,
    nav: true,
    scope: 'global',
    icon: 'compass',
  },
  {
    path: 'namespaces',
    label: 'Namespaces',
    element: <NamespacesPage />,
    nav: true,
    scope: 'global',
    icon: 'world',
  },
  {
    path: 'namespaces/new',
    label: 'Create Namespace',
    element: <NamespaceDetailPage />,
  },
  {
    path: 'namespaces/:ns/settings',
    label: 'Edit Namespace',
    element: <NamespaceDetailPage />,
    nav: true,
    scope: 'namespace',
    icon: 'settings',
  },
  {
    path: 'namespaces/:ns/catalog/items',
    label: 'Catalog Items',
    element: <CatalogItemsPage />,
    nav: true,
    scope: 'namespace',
    icon: 'image',
  },
]

export const globalNavRoutes = adminRoutes.filter(r => r.nav && r.scope === 'global')
export const namespaceNavRoutes = adminRoutes.filter(r => r.nav && r.scope === 'namespace')
