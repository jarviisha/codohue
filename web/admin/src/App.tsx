import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Layout from './components/Layout'
import LoginPage from './pages/LoginPage'
import HealthPage from './pages/HealthPage'
import NamespacesPage from './pages/NamespacesPage'
import NamespaceDetailPage from './pages/NamespaceDetailPage'
import RecommendDebugPage from './pages/RecommendDebugPage'
import BatchRunsPage from './pages/BatchRunsPage'
import TrendingPage from './pages/TrendingPage'
import EventsPage from './pages/EventsPage'

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 10_000 } },
})

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<Layout />}>
            <Route index element={<Navigate to="/health" replace />} />
            <Route path="health" element={<HealthPage />} />
            <Route path="namespaces" element={<NamespacesPage />} />
            <Route path="namespaces/new" element={<NamespaceDetailPage />} />
            <Route path="namespaces/:ns" element={<NamespaceDetailPage />} />
            <Route path="debug" element={<RecommendDebugPage />} />
            <Route path="batch-runs" element={<BatchRunsPage />} />
            <Route path="trending" element={<TrendingPage />} />
            <Route path="events" element={<EventsPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
