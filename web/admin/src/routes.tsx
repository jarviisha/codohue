import type { ReactNode } from 'react'
import HealthPage from './pages/HealthPage'
import NamespacesPage from './pages/NamespacesPage'
import NamespaceDetailPage from './pages/NamespaceDetailPage'
import RecommendDebugPage from './pages/RecommendDebugPage'
import BatchRunsPage from './pages/BatchRunsPage'
import TrendingPage from './pages/TrendingPage'
import EventsPage from './pages/EventsPage'

export interface AdminRoute {
  path: string
  label: string
  section?: string
  element: ReactNode
  nav?: boolean
}

export const adminRoutes: AdminRoute[] = [
  {
    path: 'health',
    label: 'System Health',
    section: 'System',
    element: <HealthPage />,
    nav: true,
  },
  {
    path: 'namespaces',
    label: 'Namespaces',
    section: 'Config',
    element: <NamespacesPage />,
    nav: true,
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
    path: 'batch-runs',
    label: 'Batch Runs',
    section: 'Operations',
    element: <BatchRunsPage />,
    nav: true,
  },
  {
    path: 'events',
    label: 'Events',
    section: 'Operations',
    element: <EventsPage />,
    nav: true,
  },
  {
    path: 'trending',
    label: 'Trending',
    section: 'Operations',
    element: <TrendingPage />,
    nav: true,
  },
  {
    path: 'debug',
    label: 'Recommend Debug',
    section: 'Operations',
    element: <RecommendDebugPage />,
    nav: true,
  },
]

export const navSections = ['System', 'Config', 'Operations'] as const

export function navRoutesForSection(section: string): AdminRoute[] {
  return adminRoutes.filter(route => route.nav && route.section === section)
}
