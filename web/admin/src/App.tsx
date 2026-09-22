import { lazy, Suspense, type ComponentType } from 'react'
import { createBrowserRouter, Outlet, RouterProvider } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LinkProvider, Skeleton, Theme } from '@astryxdesign/core'
import { neutralTheme } from '@astryxdesign/theme-neutral/built'
import AppShellLayout from '@/components/shell/AppShellLayout'
import { AuthGuard } from '@/components/shell/AuthGuard'
import RouterLink from '@/components/RouterLink'
import { useThemeMode } from '@/services/themeMode'

// Pages are split out of the main bundle so a cold visit only ships the
// shell + the entered route. Login is bundled directly because every cold
// visit hits it; the shell's children all defer.
import LoginPage from '@/pages/login/LoginPage'

const BatchRunDetailPage = lazy(() => import('@/pages/batch-runs/BatchRunDetailPage'))
const BatchRunsListPage = lazy(() => import('@/pages/batch-runs/BatchRunsListPage'))
const DangerZonePage = lazy(() => import('@/pages/danger-zone/DangerZonePage'))
const DemoDataPage = lazy(() => import('@/pages/demo-data/DemoDataPage'))
const FleetOverviewPage = lazy(() => import('@/pages/fleet/FleetOverviewPage'))
const RuntimePage = lazy(() => import('@/pages/system/RuntimePage'))
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
const NotFoundPage = lazy(() => import('@/pages/not-found/NotFoundPage'))

// withSuspense wraps a lazy page in a Suspense boundary so each route gets
// its own loading fallback. The fallback is intentionally generic — pages
// render their own skeletons after the chunk loads.
function withSuspense(Page: ComponentType) {
  return (
    <Suspense fallback={<Skeleton height={192} className="m-6" />}>
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
      { path: 'system/runtime', element: withSuspense(RuntimePage) },
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
          // Unmatched sub-paths under a namespace resolve here rather than
          // escaping the shell to React Router's default error screen.
          { path: '*', element: withSuspense(NotFoundPage) },
        ],
      },
      { path: '*', element: withSuspense(NotFoundPage) },
    ],
  },
])

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Default retry off for 4xx — auth failures should bubble immediately.
      // Real services override retry on a per-query basis when appropriate.
      retry: false,
      refetchOnWindowFocus: false,
    },
  },
})

/**
 * App owns the provider stack as well as the router: Theme needs the persisted
 * colour mode, which only a hook can read, and LinkProvider hands every Astryx
 * link (Button href, Breadcrumbs, SideNav) to React Router instead of letting
 * it fall back to a full-page navigation.
 */
export default function App() {
  const { mode } = useThemeMode()
  return (
    <Theme theme={neutralTheme} mode={mode}>
      <LinkProvider component={RouterLink}>
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </LinkProvider>
    </Theme>
  )
}
