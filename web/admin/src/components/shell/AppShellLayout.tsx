import { Outlet, useLocation, useNavigate, Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import {
  AppShell,
  AppShellHeader,
  AppShellMain,
  AppShellSidebar,
  AppShellTopBar,
  Avatar,
  Badge,
  Breadcrumbs,
  BreadcrumbsCurrent,
  BreadcrumbsItem,
  BreadcrumbsLink,
  BreadcrumbsList,
  BreadcrumbsSeparator,
  Button,
  DropdownMenu,
  DropdownMenuItem,
  DropdownMenuSeparator,
  Inline,
  SearchInput,
  Skeleton,
  Stack,
} from '@jarviisha/davinci-react-ui'
import { useTheme, type Theme } from '@jarviisha/davinci-react-theme-provider'
import { useLogout, useSession } from '@/services/auth'
import { recordRecentNamespace } from '@/services/recentNamespaces'
import SidebarNav from '@/components/shell/SidebarNav'
import { PageHeaderSlotContext } from '@/components/shell/pageHeaderSlot'
import ReembedOverlay from '@/components/shell/ReembedOverlay'
import RouteErrorBoundary from '@/components/shell/ErrorBoundary'
import OpsToastBridge from '@/components/shell/OpsToastBridge'

/**
 * AppShellLayout uses the Davinci AppShell "global top bar" pattern:
 * AppShellTopBar spans the full width above sidebar+main and carries
 * cluster-wide chrome (brand, command palette trigger, theme, account).
 * Page-level location lives in AppShellHeader so each page controls its
 * own breadcrumb without fighting the global bar.
 *
 *   ┌─────────────────────────── top-bar ───────────────────────────┐
 *   │  codohue            [⌘K]          [theme] [account]           │
 *   ├──────────────┬────────────────────────────────────────────────┤
 *   │              │  header (breadcrumbs)                          │
 *   │  sidebar     ├────────────────────────────────────────────────┤
 *   │              │  main (route outlet)                           │
 *   └──────────────┴────────────────────────────────────────────────┘
 */
export default function AppShellLayout() {
  const session = useSession()
  const navigate = useNavigate()
  const logout = useLogout()
  const location = useLocation()
  // PageHeader portal target — ref callback re-renders consumers via state
  // when the slot mounts (avoids first-paint flash of empty header).
  const [pageHeaderSlot, setPageHeaderSlot] = useState<HTMLDivElement | null>(null)

  useEffect(() => {
    const handler = () => {
      navigate(`/login?next=${encodeURIComponent(location.pathname)}`, { replace: true })
    }
    window.addEventListener('codohue:auth-expired', handler)
    return () => window.removeEventListener('codohue:auth-expired', handler)
  }, [navigate, location.pathname])

  // Record /ns/{name} visits so the Sidebar "Recent" group + breadcrumb
  // dropdown surface frequently-visited namespaces without forcing operators
  // back through the full /namespaces list.
  useEffect(() => {
    const match = location.pathname.match(/^\/ns\/([^/]+)/)
    if (match) {
      recordRecentNamespace(decodeURIComponent(match[1]))
    }
  }, [location.pathname])

  if (session.isLoading) {
    return <Skeleton className="h-screen w-full" />
  }

  return (
    <AppShell>
      <AppShellTopBar>
        <Link
          to="/"
          className="text-foreground font-semibold tracking-tight no-underline"
        >
          codohue
        </Link>
        <GlobalSearch />
        <Inline gap="100" align="center">
          <ThemeMenu />
          <AccountMenu
            onSignOut={() =>
              logout.mutate(undefined, {
                onSuccess: () => navigate('/login', { replace: true }),
              })
            }
            signingOut={logout.isPending}
          />
        </Inline>
      </AppShellTopBar>

      <AppShellSidebar>
        <SidebarNav />
      </AppShellSidebar>

      <AppShellHeader>
        <Stack gap="100" className="w-full">
          <RouteBreadcrumbs pathname={location.pathname} />
          <div ref={setPageHeaderSlot} className="page-header-slot" />
        </Stack>
      </AppShellHeader>

      <AppShellMain>
        <PageHeaderSlotContext.Provider value={pageHeaderSlot}>
          <RouteErrorBoundary resetKey={location.pathname}>
            <Outlet />
          </RouteErrorBoundary>
        </PageHeaderSlotContext.Provider>
      </AppShellMain>

      <ReembedOverlay />
      <OpsToastBridge />
    </AppShell>
  )
}

/**
 * GlobalSearch is the cluster-wide search input that lives in the top bar.
 * Submit (Enter) navigates to a search results route that page-level handlers
 * will wire in Phase 1+. For now the search is a no-op on submit, but the
 * input itself is real (controlled value + clear) so operators can verify the
 * affordance is keyboard-reachable.
 */
function GlobalSearch() {
  const [value, setValue] = useState('')
  return (
    <form
      role="search"
      className="flex-1 max-w-md"
      onSubmit={(event) => {
        event.preventDefault()
        /* TODO: navigate to search results (Phase 1+) */
      }}
    >
      <SearchInput
        size="sm"
        placeholder="Search namespaces, runs, items…"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onClear={() => setValue('')}
        aria-label="Global search"
      />
    </form>
  )
}

function ThemeMenu() {
  const { theme, resolvedTheme, setTheme } = useTheme()
  const options: Array<{ value: Theme; label: string }> = [
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
    { value: 'system', label: 'System' },
  ]
  return (
    <DropdownMenu
      trigger={
        <Button variant="ghost" tone="neutral" size="sm" aria-label="Theme">
          <Badge variant="neutral">{resolvedTheme}</Badge>
        </Button>
      }
    >
      {options.map((o) => (
        <DropdownMenuItem
          key={o.value}
          onClick={() => setTheme(o.value)}
          aria-current={theme === o.value || undefined}
        >
          {o.label}
          {theme === o.value && (
            <span className="text-foreground-subtle ml-2">•</span>
          )}
        </DropdownMenuItem>
      ))}
    </DropdownMenu>
  )
}

/**
 * RouteBreadcrumbs derives the breadcrumb trail from the URL — Fleet is the
 * home anchor, then each path segment becomes a crumb. The namespace switcher
 * lives in the sidebar (not the breadcrumb), so the namespace segment renders
 * as a plain link back to the namespace overview.
 *
 *   - `/ns/{name}` collapses to a single crumb labelled `{name}` linking to
 *     `/ns/{name}` (skips the literal "ns" segment).
 *   - Numeric segments (batch-run id) prefix with "#".
 */
function RouteBreadcrumbs({ pathname }: { pathname: string }) {
  const segments = pathname.split('/').filter(Boolean)
  type Crumb = { label: string; to?: string }
  const crumbs: Crumb[] = [{ label: 'fleet', to: '/' }]
  for (let i = 0; i < segments.length; i++) {
    const raw = segments[i]
    if (raw === 'ns' && segments[i + 1]) {
      const ns = segments[i + 1]
      crumbs.push({ label: ns, to: `/ns/${ns}` })
      i++
      continue
    }
    const path = '/' + segments.slice(0, i + 1).join('/')
    const label = /^\d+$/.test(raw) ? `#${raw}` : raw
    crumbs.push({ label, to: path })
  }

  return (
    <Breadcrumbs>
      <BreadcrumbsList>
        {crumbs.map((c, i) => {
          const isLast = i === crumbs.length - 1
          return (
            <BreadcrumbsItem key={`${c.label}-${i}`}>
              {isLast || !c.to ? (
                <BreadcrumbsCurrent>{c.label}</BreadcrumbsCurrent>
              ) : (
                <>
                  <BreadcrumbsLink href={c.to}>{c.label}</BreadcrumbsLink>
                  <BreadcrumbsSeparator />
                </>
              )}
            </BreadcrumbsItem>
          )
        })}
      </BreadcrumbsList>
    </Breadcrumbs>
  )
}

function AccountMenu({
  onSignOut,
  signingOut,
}: {
  onSignOut: () => void
  signingOut: boolean
}) {
  return (
    <DropdownMenu
      trigger={
        <Button
          variant="ghost"
          tone="neutral"
          size="sm"
          aria-label="Account"
          disabled={signingOut}
        >
          <Avatar size="sm" />
        </Button>
      }
    >
      <DropdownMenuItem disabled>admin</DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem onClick={onSignOut}>
        {signingOut ? 'Signing out…' : 'Sign out'}
      </DropdownMenuItem>
    </DropdownMenu>
  )
}
