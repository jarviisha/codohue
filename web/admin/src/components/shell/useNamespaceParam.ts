import { useMemo } from 'react'
import { useLocation } from 'react-router-dom'

/**
 * useNamespaceParam reads the active namespace off the URL.
 *
 * It exists because `useParams()` does not work here. React Router builds each
 * route element's context from `matches.slice(0, index + 1)`, so a component
 * rendered *inside* a layout route only sees params matched up to that layout —
 * and the shell (TopNav switcher, SideNav) lives in the `path: '/'` layout,
 * above where `ns/:ns` is matched. `useParams()` there returns `{}` on every
 * route, including `/ns/foo`.
 *
 * Reading the pathname sidesteps the context entirely, which is what the
 * command palette and the recent-namespace recorder already do.
 */
export default function useNamespaceParam(): string | null {
  const location = useLocation()
  return useMemo(() => {
    const m = location.pathname.match(/^\/ns\/([^/]+)/)
    return m ? decodeURIComponent(m[1]) : null
  }, [location.pathname])
}
