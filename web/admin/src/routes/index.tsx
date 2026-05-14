import { Routes, Route } from 'react-router-dom'
import AppShell from '@/components/layout/AppShell'
import LoginPage from '@/pages/login/LoginPage'
import HealthPage from '@/pages/health/HealthPage'
import NamespacesListPage from '@/pages/namespaces/ListPage'
import NamespaceCreatePage from '@/pages/namespaces/CreatePage'
import NamespaceLayout from '@/pages/ns/NamespaceLayout'
import OverviewPage from '@/pages/ns/OverviewPage'
import ConfigPage from '@/pages/ns/ConfigPage'
import CatalogLayout from '@/pages/ns/catalog/CatalogLayout'
import CatalogStatusPage from '@/pages/ns/catalog/StatusPage'
import CatalogConfigPage from '@/pages/ns/catalog/ConfigPage'
import CatalogItemsPage from '@/pages/ns/catalog/items/ItemsPage'
import CatalogItemDetailModal from '@/pages/ns/catalog/items/DetailModal'
import EventsListPage from '@/pages/ns/events/ListPage'
import TrendingPage from '@/pages/ns/trending/Page'
import BatchRunsListPage from '@/pages/ns/batch-runs/ListPage'
import DebugPage from '@/pages/ns/debug/Page'
import DemoDataPage from '@/pages/ns/demo-data/Page'
import KitchenSinkPage from '@/pages/_kitchen-sink/Page'

// Route declarations. The `/login` route renders outside AppShell; every other
// route shares the shell (Sidebar + TopBar + content slot).
//
// Namespace-scoped routes live under <NamespaceLayout>; child components read
// :name via useParams. See BUILD_PLAN.md §3 for the full route table.
export default function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<AppShell />}>
        <Route index element={<HealthPage />} />
        <Route path="_kitchen-sink" element={<KitchenSinkPage />} />
        <Route path="namespaces" element={<NamespacesListPage />} />
        <Route path="namespaces/new" element={<NamespaceCreatePage />} />
        <Route path="ns/:name" element={<NamespaceLayout />}>
          <Route index element={<OverviewPage />} />
          <Route path="config" element={<ConfigPage />} />
          <Route path="catalog" element={<CatalogLayout />}>
            <Route index element={<CatalogStatusPage />} />
            <Route path="config" element={<CatalogConfigPage />} />
            <Route path="items" element={<CatalogItemsPage />}>
              <Route path=":id" element={<CatalogItemDetailModal />} />
            </Route>
          </Route>
          <Route path="events" element={<EventsListPage />} />
          <Route path="trending" element={<TrendingPage />} />
          <Route path="batch-runs" element={<BatchRunsListPage />} />
          <Route path="debug" element={<DebugPage />} />
          <Route path="demo-data" element={<DemoDataPage />} />
        </Route>
      </Route>
    </Routes>
  )
}
