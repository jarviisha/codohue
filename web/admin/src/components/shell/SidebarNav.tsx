import { useMemo } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  Combobox,
  Divider,
  Nav,
  NavGroup,
  NavItem,
  Stack,
} from '@jarviisha/davinci-react-ui'
import { useNamespaces } from '@/services/namespaces'
import { useRecentNamespaces } from '@/services/recentNamespaces'

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
]

function namespaceEntries(ns: string): NavEntry[] {
  return [
    { label: 'Overview', to: `/ns/${ns}` },
    { label: 'Batch runs', to: `/ns/${ns}/batch-runs`, matchPrefixes: [`/ns/${ns}/batch-runs`] },
    { label: 'Catalog', to: `/ns/${ns}/catalog`, matchPrefixes: [`/ns/${ns}/catalog`] },
    // Phase 3+ will add: Config, Events, Trending, Debug, Demo data.
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
 * SidebarNav has two modes driven by the route:
 *
 *   - Global mode (no namespace in URL) — fleet-level nav: Fleet, Namespaces,
 *     Batch runs, Health.
 *   - Namespace mode (`/ns/:ns/*`) — the whole sidebar is scoped to that
 *     namespace. A switcher Combobox at the top lets operators jump to a
 *     sibling namespace without leaving the current sub-page; a "Back to
 *     fleet" link exits namespace mode and returns to the global view.
 *
 * The split matches the typical admin workflow: drill into one namespace to
 * debug, stay there, exit explicitly when done. Global controls don't compete
 * for sidebar space while namespace context is active.
 */
export default function SidebarNav() {
  const { ns } = useParams<{ ns?: string }>()

  if (ns) {
    return <NamespaceSidebar currentNs={ns} />
  }
  return <GlobalSidebar />
}

function GlobalSidebar() {
  const location = useLocation()
  const navigate = useNavigate()

  return (
    <Nav className="px-3 py-4">
      <NavGroup label="Global">
        {GLOBAL_ENTRIES.map((entry) => (
          <NavItem
            key={entry.to}
            active={isActive(location.pathname, entry)}
            onClick={() => navigate(entry.to)}
          >
            {entry.label}
          </NavItem>
        ))}
      </NavGroup>
    </Nav>
  )
}

function NamespaceSidebar({ currentNs }: { currentNs: string }) {
  const location = useLocation()
  const navigate = useNavigate()
  const nsList = useNamespaces()
  const recents = useRecentNamespaces()

  // Sub-path retained on namespace switch: everything after /ns/{currentNs},
  // truncated at the first id-shaped (numeric) segment so cross-namespace
  // jumps don't carry ids unique to the source ns.
  const subPath = useMemo(() => {
    const segs = location.pathname.split('/').filter(Boolean)
    if (segs[0] !== 'ns' || segs[1] !== currentNs) return ''
    const after = segs.slice(2)
    const firstNumIdx = after.findIndex((s) => /^\d+$/.test(s))
    const safe = firstNumIdx >= 0 ? after.slice(0, firstNumIdx) : after
    return safe.length > 0 ? '/' + safe.join('/') : ''
  }, [location.pathname, currentNs])

  // Options for the switcher — recent-first (excluding current), then the
  // rest alphabetically. Operators jumping between two ns get a one-keystroke
  // experience; new ns still discoverable via typeahead.
  const options = useMemo(() => {
    const all = nsList.data?.items.map((n) => n.namespace) ?? [currentNs]
    const set = new Set(all)
    const recentFirst = recents.filter((r) => r !== currentNs && set.has(r))
    const rest = all.filter((n) => n !== currentNs && !recentFirst.includes(n)).sort()
    const ordered = [currentNs, ...recentFirst, ...rest]
    return ordered.map((n) => ({ value: n, label: n }))
  }, [nsList.data, recents, currentNs])

  const switchTo = (nextNs: string) => {
    if (!nextNs || nextNs === currentNs) return
    navigate(`/ns/${encodeURIComponent(nextNs)}${subPath}`)
  }

  return (
    <Nav className="px-3 py-4">
      <Stack gap="100">
        <NavItem onClick={() => navigate('/')}>← Fleet</NavItem>
        <div className="px-3">
          <Combobox
            size="sm"
            value={currentNs}
            onValueChange={switchTo}
            options={options}
            aria-label="Switch namespace"
          />
        </div>
        <Divider />
      </Stack>

      <NavGroup label="Namespace">
        {namespaceEntries(currentNs).map((entry) => (
          <NavItem
            key={entry.to}
            active={isActive(location.pathname, entry)}
            onClick={() => navigate(entry.to)}
          >
            {entry.label}
          </NavItem>
        ))}
      </NavGroup>
    </Nav>
  )
}
