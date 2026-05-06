import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Layout from './components/Layout'
import LoginPage from './pages/LoginPage'
import { adminRoutes } from './routes'
import { NamespaceProvider } from './context/NamespaceContext'

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 10_000 } },
})

function DefaultRedirect() {
  return <Navigate to="/health" replace />
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
