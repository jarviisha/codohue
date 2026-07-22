import { lazy, Suspense, type ComponentType } from 'react'
import { createBrowserRouter, Outlet, RouterProvider } from 'react-router-dom'
import { Skeleton } from '@jarviisha/davinci-react-ui'
import AppShellLayout from '@/components/shell/AppShellLayout'
import { AuthGuard } from '@/components/shell/AuthGuard'

// Pages are split out of the main bundle so a cold visit only ships the
// shell + the entered route. Login is bundled directly because every cold
// visit hits it; the shell's children all defer.
import LoginPage from '@/pages/login/LoginPage'

const BatchRunDetailPage = lazy(() => import('@/pages/batch-runs/BatchRunDetailPage'))
const BatchRunsListPage = lazy(() => import('@/pages/batch-runs/BatchRunsListPage'))
const DangerZonePage = lazy(() => import('@/pages/danger-zone/DangerZonePage'))
const DemoDataPage = lazy(() => import('@/pages/demo-data/DemoDataPage'))
const FleetOverviewPage = lazy(() => import('@/pages/fleet/FleetOverviewPage'))
const HealthPage = lazy(() => import('@/pages/health/HealthPage'))
const NamespacesListPage = lazy(() => import('@/pages/namespaces/NamespacesListPage'))
const CatalogItemDetailPage = lazy(() => import('@/pages/ns/catalog/CatalogItemDetailPage'))
const CatalogItemsPage = lazy(() => import('@/pages/ns/catalog/CatalogItemsPage'))
const CatalogStatusPage = lazy(() => import('@/pages/ns/catalog/CatalogStatusPage'))
const EventsPage = lazy(() => import('@/pages/ns/events/EventsPage'))
const SubjectInspectorPage = lazy(() => import('@/pages/ns/subjects/SubjectInspectorPage'))
const SubjectsListPage = lazy(() => import('@/pages/ns/subjects/SubjectsListPage'))
const TrendingPage = lazy(() => import('@/pages/ns/trending/TrendingPage'))
const NamespaceConfigPage = lazy(() => import('@/pages/ns/config/NamespaceConfigPage'))
const NamespaceOverviewPage = lazy(() => import('@/pages/ns/NamespaceOverviewPage'))

// withSuspense wraps a lazy page in a Suspense boundary so each route gets
// its own loading fallback. The fallback is intentionally generic — pages
// render their own skeletons after the chunk loads.
function withSuspense(Page: ComponentType) {
  return (
    <Suspense fallback={<Skeleton className="h-48 w-full m-6" />}>
      <Page />
    </Suspense>
  )
}

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
      { index: true, element: withSuspense(FleetOverviewPage) },
      { path: 'health', element: withSuspense(HealthPage) },
      { path: 'namespaces', element: withSuspense(NamespacesListPage) },
      { path: 'batch-runs', element: withSuspense(BatchRunsListPage) },
      { path: 'batch-runs/:id', element: withSuspense(BatchRunDetailPage) },
      { path: 'demo-data', element: withSuspense(DemoDataPage) },
      { path: 'danger-zone', element: withSuspense(DangerZonePage) },
      {
        path: 'ns/:ns',
        element: <Outlet />,
        children: [
          { index: true, element: withSuspense(NamespaceOverviewPage) },
          { path: 'batch-runs', element: withSuspense(BatchRunsListPage) },
          { path: 'batch-runs/:id', element: withSuspense(BatchRunDetailPage) },
          { path: 'catalog', element: withSuspense(CatalogStatusPage) },
          { path: 'catalog/items', element: withSuspense(CatalogItemsPage) },
          { path: 'catalog/items/:id', element: withSuspense(CatalogItemDetailPage) },
          { path: 'subjects', element: withSuspense(SubjectsListPage) },
          { path: 'subjects/:id', element: withSuspense(SubjectInspectorPage) },
          { path: 'events', element: withSuspense(EventsPage) },
          { path: 'trending', element: withSuspense(TrendingPage) },
          { path: 'config', element: withSuspense(NamespaceConfigPage) },
        ],
      },
    ],
  },
])

export default function App() {
  return <RouterProvider router={router} />
}
