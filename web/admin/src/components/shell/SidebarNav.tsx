import { useLocation } from 'react-router-dom'
import { SideNav, SideNavItem, SideNavSection, useAppShellMobile } from '@astryxdesign/core'
import useNamespaceParam from '@/components/shell/useNamespaceParam'

type NavEntry = {
  label: string
  to: string
  /**
   * When provided, the nav item is active iff the current path starts with
   * one of the prefixes (in addition to an exact match on `to`). Used for
   * parents whose children share a path segment (e.g. /batch-runs covers
   * /batch-runs/:id).
   */
  matchPrefixes?: string[]
}

const GLOBAL_ENTRIES: NavEntry[] = [
  { label: 'Fleet', to: '/' },
  { label: 'Namespaces', to: '/namespaces', matchPrefixes: ['/namespaces'] },
  { label: 'Batch runs', to: '/batch-runs', matchPrefixes: ['/batch-runs'] },
  { label: 'Health', to: '/health' },
  { label: 'System runtime', to: '/system/runtime' },
  { label: 'Demo data', to: '/demo-data' },
  { label: 'Danger zone', to: '/danger-zone' },
]

function namespaceEntries(ns: string): NavEntry[] {
  return [
    { label: 'Overview', to: `/ns/${ns}` },
    { label: 'Batch runs', to: `/ns/${ns}/batch-runs`, matchPrefixes: [`/ns/${ns}/batch-runs`] },
    { label: 'Catalog', to: `/ns/${ns}/catalog`, matchPrefixes: [`/ns/${ns}/catalog`] },
    { label: 'Subjects', to: `/ns/${ns}/subjects`, matchPrefixes: [`/ns/${ns}/subjects`] },
    { label: 'Events', to: `/ns/${ns}/events`, matchPrefixes: [`/ns/${ns}/events`] },
    { label: 'Trending', to: `/ns/${ns}/trending` },
    { label: 'Configuration', to: `/ns/${ns}/config` },
  ]
}

function isActive(pathname: string, entry: NavEntry): boolean {
  if (pathname === entry.to) return true
  if (entry.matchPrefixes) {
    return entry.matchPrefixes.some((p) => pathname === p || pathname.startsWith(`${p}/`))
  }
  return false
}

/**
 * SidebarNav carries product navigation only — the namespace context switcher
 * lives in the TopNav (see NamespaceSwitcher).
 *
 * The active URL selects one scope: global destinations outside a namespace,
 * or namespace destinations inside one. A persistent return link restores the
 * global navigation without mixing two sets of actions in the same sidebar.
 *
 * On narrow viewports AppShell renders this inside its mobile drawer. AppShell
 * knows nothing about the router, so it cannot close that drawer when a route
 * changes — tapping an entry would navigate and leave the drawer covering the
 * page. Hence the explicit closeMobileNav() after each navigation.
 */
export default function SidebarNav() {
  const ns = useNamespaceParam()
  const location = useLocation()
  const { closeMobileNav } = useAppShellMobile()

  const renderEntry = (entry: NavEntry) => (
    <SideNavItem
      key={entry.to}
      label={entry.label}
      isSelected={isActive(location.pathname, entry)}
      href={entry.to}
      onClick={() => closeMobileNav()}
    />
  )

  return (
    <SideNav aria-label="Main navigation">
      {ns ? (
        <>
          <SideNavItem
            label="← All namespaces"
            href="/namespaces"
            onClick={() => closeMobileNav()}
          />
          <SideNavSection title="Namespace" subtitle={ns}>
            {namespaceEntries(ns).map(renderEntry)}
          </SideNavSection>
        </>
      ) : (
        <>
          <SideNavSection title="Global">{GLOBAL_ENTRIES.slice(0, 5).map(renderEntry)}</SideNavSection>
          <SideNavSection title="Tools">{GLOBAL_ENTRIES.slice(5).map(renderEntry)}</SideNavSection>
        </>
      )}
    </SideNav>
  )
}
