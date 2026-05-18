import { useEffect } from 'react'
import { matchPath, useLocation } from 'react-router-dom'

const APP_NAME = 'Codohue Admin'

// Single source of truth for tab titles. Mounted once at App-level so no page
// opts in individually; covers shell pages, /login, and the catch-all
// not-found route. The entries below mirror the patterns in
// [routes/index.tsx] — add a row here when adding a route there.

type TitleResolver = string | ((params: Record<string, string | undefined>) => string)

interface TitleRoute {
  pattern: string // react-router path pattern (no leading slash semantics)
  title: TitleResolver
}

// Order matters: the first pattern that matches wins. Put the more specific
// routes first when prefixes overlap.
const TITLE_ROUTES: TitleRoute[] = [
  { pattern: '/login', title: 'Login' },
  { pattern: '/namespaces/new', title: 'Create namespace' },
  { pattern: '/namespaces', title: 'Namespaces' },
  { pattern: '/_kitchen-sink', title: 'Kitchen sink' },
  { pattern: '/ns/:name/catalog/items/:id', title: ({ name }) => withNs('Catalog item', name) },
  { pattern: '/ns/:name/catalog/items', title: ({ name }) => withNs('Catalog items', name) },
  { pattern: '/ns/:name/catalog/config', title: ({ name }) => withNs('Catalog config', name) },
  { pattern: '/ns/:name/catalog', title: ({ name }) => withNs('Catalog', name) },
  { pattern: '/ns/:name/events', title: ({ name }) => withNs('Events', name) },
  { pattern: '/ns/:name/trending', title: ({ name }) => withNs('Trending', name) },
  { pattern: '/ns/:name/batch-runs/re-embeds', title: ({ name }) => withNs('Re-embeds', name) },
  { pattern: '/ns/:name/batch-runs', title: ({ name }) => withNs('Batch runs', name) },
  { pattern: '/ns/:name/debug', title: ({ name }) => withNs('Recommend debug', name) },
  { pattern: '/ns/:name/demo-data', title: ({ name }) => withNs('Demo data', name) },
  { pattern: '/ns/:name/config', title: ({ name }) => withNs('Config', name) },
  { pattern: '/ns/:name', title: ({ name }) => withNs('Overview', name) },
  { pattern: '/', title: 'Health' },
]

function withNs(label: string, name?: string): string {
  return name ? `${label} · ${name}` : label
}

function resolveTitle(pathname: string): string {
  for (const route of TITLE_ROUTES) {
    const match = matchPath({ path: route.pattern, end: true }, pathname)
    if (!match) continue
    const params = match.params as Record<string, string | undefined>
    const label = typeof route.title === 'function' ? route.title(params) : route.title
    return `${label} · ${APP_NAME}`
  }
  // Nothing matched — caught by the catch-all `<Route path="*">` in
  // routes/index.tsx, which renders NotFoundPage.
  return `Not found · ${APP_NAME}`
}

// Side-effect-only hook. Mounted once in App.tsx so every pathname change
// updates the tab title without per-page boilerplate.
export default function useDocumentTitle() {
  const { pathname } = useLocation()
  useEffect(() => {
    document.title = resolveTitle(pathname)
  }, [pathname])
}
