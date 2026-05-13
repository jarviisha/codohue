import { Outlet, useParams } from 'react-router-dom'

// Wraps namespace-scoped routes. Renders <Outlet /> for child routes; sidebar
// and Ps1Prompt independently read :name from useLocation.
//
// Phase 2 can grow this to inject a NamespaceContext-equivalent slot if needed,
// but per BUILD_PLAN.md §4.3 the URL is the only source of truth — child
// components call useParams<{ name }>() directly.
export default function NamespaceLayout() {
  const { name } = useParams<{ name: string }>()
  if (!name) return null
  return <Outlet />
}
