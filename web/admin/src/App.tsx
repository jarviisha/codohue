import useDocumentTitle from '@/components/layout/useDocumentTitle'
import AppRoutes from './routes'

// Top-level component inside <BrowserRouter>. Mounts the global
// document.title hook here (one mount point covers /login, the AppShell
// routes, and the catch-all not-found) and renders the route tree.
export default function App() {
  useDocumentTitle()
  return <AppRoutes />
}
