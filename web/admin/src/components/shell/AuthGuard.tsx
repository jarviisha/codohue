import { Navigate, useLocation } from 'react-router-dom'
import { Skeleton } from '@jarviisha/davinci-react-ui'
import { useSession } from '@/services/auth'
import type { ReactNode } from 'react'

/**
 * AuthGuard probes the admin session and either renders the protected subtree
 * or redirects to /login with a `next` query string so login can bounce back
 * to the original target.
 */
export function AuthGuard({ children }: { children: ReactNode }) {
  const session = useSession()
  const location = useLocation()

  if (session.isLoading) {
    return <Skeleton className="h-screen w-full" />
  }
  if (session.isError) {
    return <Navigate to={`/login?next=${encodeURIComponent(location.pathname)}`} replace />
  }
  return <>{children}</>
}
