import { createBrowserRouter, Outlet, RouterProvider } from 'react-router-dom'
import AppShellLayout from '@/components/shell/AppShellLayout'
import { AuthGuard } from '@/components/shell/AuthGuard'
import BatchRunDetailPage from '@/pages/batch-runs/BatchRunDetailPage'
import BatchRunsListPage from '@/pages/batch-runs/BatchRunsListPage'
import FleetOverviewPage from '@/pages/fleet/FleetOverviewPage'
import HealthPage from '@/pages/health/HealthPage'
import LoginPage from '@/pages/login/LoginPage'
import CreateNamespacePage from '@/pages/namespaces/CreateNamespacePage'
import NamespacesListPage from '@/pages/namespaces/NamespacesListPage'
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
      {
        path: 'ns/:ns',
        element: <Outlet />,
        children: [
          { index: true, element: <NamespaceOverviewPage /> },
          { path: 'batch-runs', element: <BatchRunsListPage /> },
        ],
      },
    ],
  },
])

export default function App() {
  return <RouterProvider router={router} />
}
