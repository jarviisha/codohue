import { createBrowserRouter, Outlet, RouterProvider } from 'react-router-dom'
import AppShellLayout from '@/components/shell/AppShellLayout'
import { AuthGuard } from '@/components/shell/AuthGuard'
import BatchRunDetailPage from '@/pages/batch-runs/BatchRunDetailPage'
import BatchRunsListPage from '@/pages/batch-runs/BatchRunsListPage'
import DangerZonePage from '@/pages/danger-zone/DangerZonePage'
import FleetOverviewPage from '@/pages/fleet/FleetOverviewPage'
import HealthPage from '@/pages/health/HealthPage'
import LoginPage from '@/pages/login/LoginPage'
import CreateNamespacePage from '@/pages/namespaces/CreateNamespacePage'
import NamespacesListPage from '@/pages/namespaces/NamespacesListPage'
import CatalogItemDetailPage from '@/pages/ns/catalog/CatalogItemDetailPage'
import CatalogItemsPage from '@/pages/ns/catalog/CatalogItemsPage'
import CatalogStatusPage from '@/pages/ns/catalog/CatalogStatusPage'
import EventsPage from '@/pages/ns/events/EventsPage'
import SubjectInspectorPage from '@/pages/ns/subjects/SubjectInspectorPage'
import SubjectLookupPage from '@/pages/ns/subjects/SubjectLookupPage'
import TrendingPage from '@/pages/ns/trending/TrendingPage'
import NamespaceOverviewPage from '@/pages/ns/NamespaceOverviewPage'

const router = createBrowserRouter([
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    path: '/',
    element: (
      <AuthGuard>
        <AppShellLayout />
      </AuthGuard>
    ),
    children: [
      { index: true, element: <FleetOverviewPage /> },
      { path: 'health', element: <HealthPage /> },
      { path: 'namespaces', element: <NamespacesListPage /> },
      { path: 'namespaces/new', element: <CreateNamespacePage /> },
      { path: 'batch-runs', element: <BatchRunsListPage /> },
      { path: 'batch-runs/:id', element: <BatchRunDetailPage /> },
      { path: 'danger-zone', element: <DangerZonePage /> },
      {
        path: 'ns/:ns',
        element: <Outlet />,
        children: [
          { index: true, element: <NamespaceOverviewPage /> },
          { path: 'batch-runs', element: <BatchRunsListPage /> },
          { path: 'catalog', element: <CatalogStatusPage /> },
          { path: 'catalog/items', element: <CatalogItemsPage /> },
          { path: 'catalog/items/:id', element: <CatalogItemDetailPage /> },
          { path: 'subjects', element: <SubjectLookupPage /> },
          { path: 'subjects/:id', element: <SubjectInspectorPage /> },
          { path: 'events', element: <EventsPage /> },
          { path: 'trending', element: <TrendingPage /> },
        ],
      },
    ],
  },
])

export default function App() {
  return <RouterProvider router={router} />
}
