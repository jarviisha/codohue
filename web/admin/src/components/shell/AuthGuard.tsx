import { Navigate, useLocation } from 'react-router-dom'
import { Banner, Button, Skeleton, Stack } from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import { useSession } from '@/services/auth'
import { isAuthError } from '@/services/http'
import type { ReactNode } from 'react'

/**
 * AuthGuard probes the admin session and either renders the protected subtree
 * or redirects to /login with a `next` query string so login can bounce back
 * to the original target.
 *
 * Only a 401 counts as "not signed in". Every other failure — the admin plane
 * being down, a 500, a dropped connection — leaves the session untouched, so
 * redirecting on it would sign out an operator whose cookie is perfectly valid
 * and replace a diagnosable error with a login screen. Those render a retry
 * instead.
 *
 * `next` carries the search string as well as the path: filters and pagination
 * live in the query, and dropping them means the bounce-back lands on a
 * different view than the one the operator was on.
 */
export function AuthGuard({ children }: { children: ReactNode }) {
  const session = useSession()
  const location = useLocation()

  if (session.isLoading) {
    return <Skeleton className="h-screen w-full" />
  }
  if (session.isError) {
    if (isAuthError(session.error)) {
      const target = `${location.pathname}${location.search}`
      return <Navigate to={`/login?next=${encodeURIComponent(target)}`} replace />
    }
    return (
      <PageContainer size="sm">
        <Stack gap={6}>
          <Banner
            status="error"
            title="Could not reach the admin plane"
            description={`${session.error.message}. Your session is still valid — this is a connectivity or server error, not a sign-out.`}
          />
          <Button onClick={() => session.refetch()} isDisabled={session.isFetching} label={session.isFetching ? 'Retrying…' : 'Retry'} />
        </Stack>
      </PageContainer>
    )
  }
  return <>{children}</>
}
