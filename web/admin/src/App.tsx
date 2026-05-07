import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
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

  const namespaces = data?.namespaces ?? []
  const hasActiveNamespace = namespaces.some(ns => ns.namespace === namespace)

  if (hasActiveNamespace) return <Navigate to="/overview" replace />

  return <Navigate to="/namespaces" replace />
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
              {adminRoutes.map(route => (
                <Route key={route.path} path={route.path} element={route.element} />
              ))}
            </Route>
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </NamespaceProvider>
  )
}
