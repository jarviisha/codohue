import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useEffect, useState } from 'react'
import {
  AppShell,
  Avatar,
  Token,
  Button,
  Kbd,
  BreadcrumbItem,
  Breadcrumbs,
  DropdownMenu,
  Link,
  Skeleton,
  Stack,
  TopNav,
  useHotkeys,
} from '@astryxdesign/core'
import { useLogout, useSession } from '@/services/auth'
import { recordRecentNamespace } from '@/services/recentNamespaces'
import SidebarNav from '@/components/shell/SidebarNav'
import NamespaceSwitcher from '@/components/shell/NamespaceSwitcher'
import { PageHeaderSlotContext } from '@/components/shell/pageHeaderSlot'
import useNamespaceParam from '@/components/shell/useNamespaceParam'
import ReembedOverlay from '@/components/shell/ReembedOverlay'
import RouteErrorBoundary from '@/components/shell/ErrorBoundary'
import NavigationProgress from '@/components/shell/NavigationProgress'
import useNavigationPending from '@/components/shell/useNavigationPending'
import NamespaceTag from '@/components/NamespaceTag'
import OpsToastBridge from '@/components/shell/OpsToastBridge'
import CommandPalette from '@/components/shell/CommandPalette'
import { useThemeMode, type ThemeMode } from '@/services/themeMode'

/**
 * AppShellLayout is the Astryx AppShell in its two-bar form. Astryx only
 * justifies two bars when the top one carries ecosystem-wide concerns while
 * the side one carries product nav, so the split here is:
 *
 *   - TopNav: namespace context switcher + global search (⌘K) + app chrome
 *   - SideNav: destinations, nothing else
 *
 *   ┌──────────────────────────── top-nav ──────────────────────────┐
 *   │  codohue  [namespace ▾]    [⌘K]        [theme] [account]      │
 *   ├──────────────┬────────────────────────────────────────────────┤
 *   │  side-nav    │  breadcrumbs + page header slot                │
 *   │              │  route outlet                                  │
 *   └──────────────┴────────────────────────────────────────────────┘
 *
 * AppShell owns the mobile drawer and the skip link, so the media query that
 * used to swap the sidebar for a Drawer is gone. Page-level location lives
 * above the outlet so each page controls its own breadcrumb without fighting
 * the global bar.
 */
export default function AppShellLayout() {
  const session = useSession()
  const navigate = useNavigate()
  const logout = useLogout()
  const location = useLocation()
  // PageHeader portal target — ref callback re-renders consumers via state
  // when the slot mounts (avoids first-paint flash of empty header).
  const [pageHeaderSlot, setPageHeaderSlot] = useState<HTMLElement | null>(null)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const ns = useNamespaceParam()
  // Marks the outlet busy for the whole navigation, including the first 150ms
  // before NavigationProgress draws its bar.
  const { isPending: isNavigating } = useNavigationPending()

  // Cmd+K (Mac) / Ctrl+K (everywhere else) opens the command palette from any
  // focused element. useHotkeys already skips events originating in inputs,
  // which is what the hand-rolled keydown listener was working around.
  useHotkeys([{ keys: 'mod+k', onPress: () => setPaletteOpen((v) => !v) }])

  // The search string rides along so the post-login bounce lands on the same
  // view, filters and paging included, rather than its bare path.
  useEffect(() => {
    const handler = () => {
      const target = `${location.pathname}${location.search}`
      navigate(`/login?next=${encodeURIComponent(target)}`, { replace: true })
    }
    window.addEventListener('codohue:auth-expired', handler)
    return () => window.removeEventListener('codohue:auth-expired', handler)
  }, [navigate, location.pathname, location.search])

  // Record /ns/{name} visits so the Sidebar "Recent" group + breadcrumb
  // dropdown surface frequently-visited namespaces without forcing operators
  // back through the full /namespaces list.
  useEffect(() => {
    if (ns) recordRecentNamespace(ns)
  }, [ns])

  if (session.isLoading) {
    return <Skeleton height="100vh" />
  }

  return (
    <AppShell
      contentPadding={0}
      variant="section"
      topNav={
        <TopNav
          label="Global"
          heading={
            <Link href="/" hasUnderline={false} className="uppercase font-extrabold tracking-tight">
              codohue
            </Link>
          }
          startContent={<NamespaceSwitcher />}
          centerContent={<PaletteTrigger onOpen={() => setPaletteOpen(true)} />}
          endContent={
            <Stack direction="horizontal" gap={2} align="center">
              <ThemeMenu />
              <AccountMenu
                username={session.data?.actor.name ?? "Operator"}
                onSignOut={() =>
                  logout.mutate(undefined, {
                    onSuccess: () => navigate('/login', { replace: true }),
                  })
                }
                signingOut={logout.isPending}
              />
            </Stack>
          }
        />
      }
      sideNav={<SidebarNav />}
    >
      <Stack gap={4} padding={6} aria-busy={isNavigating || undefined}>
        <Stack gap={2}>
          <NavigationProgress />
          <RouteBreadcrumbs pathname={location.pathname} />
          <Stack ref={setPageHeaderSlot} />
        </Stack>

        <PageHeaderSlotContext.Provider value={pageHeaderSlot}>
          <RouteErrorBoundary resetKey={location.pathname}>
            <Outlet />
          </RouteErrorBoundary>
        </PageHeaderSlotContext.Provider>
      </Stack>

      <ReembedOverlay />
      <OpsToastBridge />
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
    </AppShell>
  )
}

/**
 * PaletteTrigger is the top-bar button that opens the command palette. It
 * mimics a search input shape so the affordance reads visually as "type to
 * jump", but it's a real button — typing happens inside the palette dialog
 * where the keyboard semantics (Arrow/Enter/Esc) live.
 */
function PaletteTrigger({ onOpen }: { onOpen: () => void }) {
  return (
    <Button
      type="button"
      variant="secondary"
      size="sm"
      onClick={onOpen}
      aria-label="Open command palette"
      label="Jump to…"
      endContent={<Kbd keys="mod+k" />}
    />
  )
}

function ThemeMenu() {
  const { mode, resolvedMode, setMode } = useThemeMode()
  const options: Array<{ value: ThemeMode; label: string }> = [
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
    { value: 'system', label: 'System' },
  ]
  return (
    <DropdownMenu
      button={{
        variant: 'ghost',
        size: 'sm',
        label: 'Theme',
        endContent: <Token label={resolvedMode} />,
      }}
      items={options.map((o) => ({
        label: o.label,
        endContent: mode === o.value ? '•' : undefined,
        onClick: () => setMode(o.value),
      }))}
    />
  )
}

/**
 * RouteBreadcrumbs derives the breadcrumb trail from the URL — Fleet is the
 * home anchor, then each path segment becomes a crumb. Switching namespace is
 * the TopNav's job, so the namespace segment here is just a link back to that
 * namespace's overview.
 *
 *   - `/ns/{name}` collapses to a single crumb labelled `{name}` linking to
 *     `/ns/{name}` (skips the literal "ns" segment).
 *   - Numeric segments (batch-run id) prefix with "#".
 *
 * Crumb links are plain hrefs: LinkProvider hands them to React Router, so
 * modified clicks and "open in new tab" keep working.
 */
function RouteBreadcrumbs({ pathname }: { pathname: string }) {
  const segments = pathname.split('/').filter(Boolean)
  type Crumb = { label: string; to?: string; isNamespace?: boolean }
  const crumbs: Crumb[] = [{ label: 'fleet', to: '/' }]
  for (let i = 0; i < segments.length; i++) {
    const raw = segments[i]
    if (raw === 'ns' && segments[i + 1]) {
      const ns = segments[i + 1]
      crumbs.push({ label: ns, to: `/ns/${ns}`, isNamespace: true })
      i++
      continue
    }
    const path = '/' + segments.slice(0, i + 1).join('/')
    const label = /^\d+$/.test(raw) ? `#${raw}` : raw
    crumbs.push({ label, to: path })
  }

  return (
    <Breadcrumbs label="Breadcrumb">
      {crumbs.map((c, i) => {
        const isLast = i === crumbs.length - 1
        return (
          <BreadcrumbItem
            key={`${c.label}-${i}`}
            href={isLast ? undefined : c.to}
            isCurrent={isLast}
          >
            {c.isNamespace ? <NamespaceTag name={c.label} /> : c.label}
          </BreadcrumbItem>
        )
      })}
    </Breadcrumbs>
  )
}

function AccountMenu({
  username,
  onSignOut,
  signingOut,
}: {
  username: string
  onSignOut: () => void
  signingOut: boolean
}) {
  return (
    <DropdownMenu
      button={{
        variant: 'ghost',
        size: 'sm',
        label: 'Account',
        isIconOnly: true,
        icon: <Avatar size="sm" />,
        isDisabled: signingOut,
      }}
      items={[
        { label: username, isDisabled: true },
        { type: 'divider' },
        { label: signingOut ? 'Signing out…' : 'Sign out', onClick: onSignOut },
      ]}
    />
  )
}
