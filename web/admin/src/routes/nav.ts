import { paths } from './path'

// PS1 helpers live in routes/ps1.mjs as plain JS so `tests/urls.test.mjs`
// can import them with `node --test` (no TypeScript runtime). Re-export
// here so existing callers keep the canonical `@/routes/nav` import.
export { parsePs1, formatPs1, segmentTo } from './ps1.mjs'

export interface NavItem {
  label: string
  to: string
  end?: boolean
}

export const globalNav: NavItem[] = [
  { label: 'Health', to: paths.health, end: true },
  { label: 'Namespaces', to: paths.namespaces, end: true },
  { label: 'Demo Data', to: paths.demoData, end: true },
  { label: 'Danger Zone', to: paths.dangerZone, end: true },
]

export function namespaceNav(name: string): NavItem[] {
  return [
    { label: 'Overview',   to: paths.ns(name), end: true },
    { label: 'Config',     to: paths.nsConfig(name) },
    { label: 'Catalog',    to: paths.nsCatalog(name) },
    { label: 'Events',     to: paths.nsEvents(name) },
    { label: 'Trending',   to: paths.nsTrending(name) },
    { label: 'Batch Runs', to: paths.nsBatchRuns(name) },
    { label: 'Debug',      to: paths.nsDebug(name) },
  ]
}
