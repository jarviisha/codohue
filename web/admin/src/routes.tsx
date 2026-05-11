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
  icon?: IconName
}

export const adminRoutes: AdminRoute[] = [
  {
    path: 'overview',
    label: 'Overview',
    element: <NamespaceOverviewPage />,
    nav: true,
    icon: 'home',
  },
  {
    path: 'batch-runs',
    label: 'Batch Runs',
    element: <BatchRunsPage />,
    nav: true,
    icon: 'clock-hour-11',
  },
  {
    path: 'events',
    label: 'Events',
    element: <EventsPage />,
    nav: true,
    icon: 'bell',
  },
  {
    path: 'trending',
    label: 'Trending',
    element: <TrendingPage />,
    nav: true,
    icon: 'arrow-big-up-line',
  },
  {
    path: 'debug',
    label: 'Recommend Debug',
    element: <RecommendDebugPage />,
    nav: true,
    icon: 'search',
  },
  {
    path: 'health',
    label: 'System Health',
    element: <HealthPage />,
    nav: true,
    icon: 'compass',
  },
  {
    path: 'namespaces',
    label: 'Namespaces',
    element: <NamespacesPage />,
  },
  {
    path: 'namespaces/new',
    label: 'Create Namespace',
    element: <NamespaceDetailPage />,
  },
  {
    path: 'namespaces/:ns',
    label: 'Edit Namespace',
    element: <NamespaceDetailPage />,
  },
  {
    path: 'namespaces/:ns/catalog/items',
    label: 'Catalog Items',
    element: <CatalogItemsPage />,
  },
]

export const navRoutes = adminRoutes.filter(r => r.nav)
