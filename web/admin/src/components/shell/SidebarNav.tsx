import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { SideNav, SideNavItem, SideNavSection } from '@astryxdesign/core'

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
    { label: 'Config', to: `/ns/${ns}/config` },
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
 * Both sections are always present in the same order: Global at the top, and
 * Namespace below it once a namespace is in the URL. The sidebar used to swap
 * its whole contents between a global and a namespace mode, which meant the
 * fleet-level destinations disappeared while you were debugging a namespace
 * and you needed a "← Fleet" escape hatch to get back. Appending instead of
 * replacing keeps every destination one click away and keeps item positions
 * stable as you drill in.
 *
 * `onNavigate` fires after any entry navigates. AppShell uses it to dismiss
 * the mobile drawer.
 */
export default function SidebarNav({ onNavigate }: { onNavigate?: () => void }) {
  const { ns } = useParams<{ ns?: string }>()
  const location = useLocation()
  const navigate = useNavigate()

  const go = (to: string) => {
    navigate(to)
    onNavigate?.()
  }

  const renderEntry = (entry: NavEntry) => (
    <SideNavItem
      key={entry.to}
      label={entry.label}
      isSelected={isActive(location.pathname, entry)}
      onClick={() => go(entry.to)}
    />
  )

  return (
    <SideNav aria-label="Main navigation">
      <SideNavSection title="Global">{GLOBAL_ENTRIES.map(renderEntry)}</SideNavSection>
      {ns && (
        <SideNavSection title="Namespace" subtitle={ns}>
          {namespaceEntries(ns).map(renderEntry)}
        </SideNavSection>
      )}
    </SideNav>
  )
}
