import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useEffect } from 'react'
import {
  AppShell,
  AppShellMain,
  AppShellSidebar,
  AppShellTopBar,
  Badge,
  Breadcrumbs,
  BreadcrumbsCurrent,
  BreadcrumbsItem,
  BreadcrumbsLink,
  BreadcrumbsList,
  BreadcrumbsSeparator,
  Button,
  Inline,
  Skeleton,
} from '@jarviisha/davinci-react-ui'
import { useTheme } from '@jarviisha/davinci-react-theme-provider'
import { useLogout, useSession } from '@/services/auth'

/**
 * AppShellLayout is the protected shell: AuthGuard wraps it on the route side,
 * so by the time we render here the session probe has already succeeded. The
 * shell is intentionally empty in Phase 0 — sidebar + breadcrumbs + theme
 * toggle wired up, but no real navigation entries yet. Real Nav entries land
 * in Phase 1 once the routes exist.
 */
export default function AppShellLayout() {
  const session = useSession()
  const navigate = useNavigate()
  const logout = useLogout()

  // Subscribe once: when http.ts dispatches `codohue:auth-expired` from a 401,
  // bounce to /login with a `next` redirect back to wherever we were.
  const location = useLocation()
  useEffect(() => {
    const handler = () => {
      navigate(`/login?next=${encodeURIComponent(location.pathname)}`, { replace: true })
    }
    window.addEventListener('codohue:auth-expired', handler)
    return () => window.removeEventListener('codohue:auth-expired', handler)
  }, [navigate, location.pathname])

  if (session.isLoading) {
    return <Skeleton className="h-screen w-full" />
  }

  return (
    <AppShell>
      <AppShellTopBar>
        <Breadcrumbs>
          <BreadcrumbsList>
            <BreadcrumbsItem>
              <BreadcrumbsLink href="/">codohue</BreadcrumbsLink>
            </BreadcrumbsItem>
            <BreadcrumbsSeparator />
            <BreadcrumbsItem>
              <BreadcrumbsCurrent>home</BreadcrumbsCurrent>
            </BreadcrumbsItem>
          </BreadcrumbsList>
        </Breadcrumbs>
        <Inline gap="100" align="center">
          <ThemeToggle />
          <Button
            variant="ghost"
            tone="neutral"
            size="sm"
            onClick={() =>
              logout.mutate(undefined, {
                onSuccess: () => navigate('/login', { replace: true }),
              })
            }
          >
            Sign out
          </Button>
        </Inline>
      </AppShellTopBar>
      <AppShellSidebar>
        <nav className="px-3 py-4 text-foreground-subtle text-sm">
          <p className="font-medium text-foreground mb-2">Global</p>
          <p>(Phase 1)</p>
        </nav>
      </AppShellSidebar>
      <AppShellMain>
        <Outlet />
      </AppShellMain>
    </AppShell>
  )
}

function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme()
  const next = resolvedTheme === 'dark' ? 'light' : 'dark'
  return (
    <Button
      variant="ghost"
      tone="neutral"
      size="sm"
      onClick={() => setTheme(next)}
      aria-label={`Switch to ${next} theme`}
    >
      <Badge variant="neutral">{resolvedTheme}</Badge>
    </Button>
  )
}
