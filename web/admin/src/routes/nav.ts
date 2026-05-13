import { paths } from './path'

export interface NavItem {
  label: string
  to: string
  end?: boolean
}

export const globalNav: NavItem[] = [
  { label: 'Health', to: paths.health, end: true },
  { label: 'Namespaces', to: paths.namespaces, end: true },
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
    { label: 'Demo Data',  to: paths.nsDemoData(name) },
  ]
}

// Parse pathname into PS1 prompt segments. Used by Ps1Prompt + Sidebar to
// decide which namespace block (if any) to render.
//
// Examples:
//   /                       -> { ns: '~', segments: [] }
//   /namespaces             -> { ns: '~', segments: ['namespaces'] }
//   /namespaces/new         -> { ns: '~', segments: ['namespaces', 'new'] }
//   /ns/prod                -> { ns: 'prod', segments: [] }
//   /ns/prod/events         -> { ns: 'prod', segments: ['events'] }
//   /ns/prod/catalog/items  -> { ns: 'prod', segments: ['catalog', 'items'] }
export function parsePs1(pathname: string): { ns: string; segments: string[] } {
  const parts = pathname.split('/').filter(Boolean)
  if (parts[0] === 'ns' && parts[1]) {
    return { ns: parts[1], segments: parts.slice(2) }
  }
  return { ns: '~', segments: parts }
}

// Build the URL for the i-th segment of a PS1 path. Used to make PS1 segments
// clickable.
export function segmentTo(ns: string, segments: string[], idx: number): string {
  const sub = segments.slice(0, idx + 1).join('/')
  return ns === '~' ? `/${sub}` : `/ns/${ns}/${sub}`
}
