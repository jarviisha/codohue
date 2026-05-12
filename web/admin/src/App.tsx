import { useEffect, type ReactNode } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useParams } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Layout from './components/Layout'
import LoginPage from './pages/LoginPage'
import { adminRoutes } from './routes'
import { NamespaceProvider } from './context/NamespaceContext'
import { useActiveNamespace } from './context/useActiveNamespace'
import { useNamespaceList } from './hooks/useNamespaces'
import { LoadingState } from './components/ui'

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 10_000 } },
})

function DefaultRedirect() {
  const { namespace } = useActiveNamespace()
  const { data, isLoading } = useNamespaceList()

  if (isLoading) return <LoadingState />

  const namespaces = data?.items ?? []
  const hasActiveNamespace = namespaces.some(ns => ns.namespace === namespace)

  if (hasActiveNamespace) return <Navigate to={`/namespaces/${encodeURIComponent(namespace)}/overview`} replace />

  return <Navigate to="/namespaces" replace />
}

function NamespaceRoute({ children }: { children: ReactNode }) {
  const { ns } = useParams()
  const { namespace, setNamespace } = useActiveNamespace()

  useEffect(() => {
    if (ns && ns !== namespace) setNamespace(ns)
  }, [namespace, ns, setNamespace])

  if (ns && ns !== namespace) return <LoadingState />

  return children
}

function LegacyNamespaceRedirect({ section }: { section: string }) {
  const { namespace } = useActiveNamespace()

  if (!namespace) return <Navigate to="/namespaces" replace />
  return <Navigate to={`/namespaces/${encodeURIComponent(namespace)}/${section}`} replace />
}

function LegacySettingsRedirect() {
  const { ns } = useParams()

  if (!ns) return <Navigate to="/namespaces" replace />
  return <Navigate to={`/namespaces/${encodeURIComponent(ns)}/settings`} replace />
}

export default function App() {
  return (
    <NamespaceProvider>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/" element={<Layout />}>
              <Route index element={<DefaultRedirect />} />
              <Route path="overview" element={<LegacyNamespaceRedirect section="overview" />} />
              <Route path="batch-runs" element={<LegacyNamespaceRedirect section="batch-runs" />} />
              <Route path="events" element={<LegacyNamespaceRedirect section="events" />} />
              <Route path="trending" element={<LegacyNamespaceRedirect section="trending" />} />
              <Route path="debug" element={<LegacyNamespaceRedirect section="debug" />} />
              <Route path="namespaces/:ns" element={<LegacySettingsRedirect />} />
              {adminRoutes.map(route => (
                <Route
                  key={route.path}
                  path={route.path}
                  element={
                    route.scope === 'namespace'
                      ? <NamespaceRoute>{route.element}</NamespaceRoute>
                      : route.element
                  }
                />
              ))}
            </Route>
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </NamespaceProvider>
  )
}
