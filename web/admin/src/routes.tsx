import { matchRoutes, Outlet, type RouteObject } from 'react-router-dom'
import type { ComponentType, ReactNode } from 'react'
import { Skeleton } from '@astryxdesign/core'
import AppShellLayout from '@/components/shell/AppShellLayout'
import { AuthGuard } from '@/components/shell/AuthGuard'
import RouteLoadError from '@/components/shell/RouteLoadError'

// Login is bundled directly because every cold visit hits it; the shell's
// children all defer so a cold visit only ships the shell + the entered route.
import LoginPage from '@/pages/login/LoginPage'

type PageModule = { default: ComponentType }
type PageLoader = () => Promise<PageModule>

/** Route handle carrying the page loader so hover/focus can warm the chunk. */
type PreloadHandle = { preload: PageLoader }

/**
 * The slice of a route that page() fills in. Deliberately narrower than
 * RouteObject: that type is a union whose index-route arm pins `index` to
 * `false`, so spreading it into `{ index: true, ...page(…) }` widens `index`
 * back to `boolean` and no longer matches either arm.
 */
type PageRoute = {
  lazy: () => Promise<{ Component: ComponentType }>
  hydrateFallbackElement: ReactNode
  handle: PreloadHandle
}

/**
 * page() hands a lazily-loaded page to React Router rather than to React.lazy.
 *
 * The difference matters for navigation feedback. With React.lazy the router
 * commits the new location immediately and React — which renders the route
 * swap inside a transition — keeps the *previous* page on screen while the
 * chunk downloads, without falling back to the Suspense boundary. The URL
 * changes, the content does not, and nothing reports that anything is
 * happening. Routing the load through React Router's own `lazy` instead makes
 * the download part of the navigation, so `useNavigation()` reports
 * state === 'loading' for its whole duration and the shell can show it.
 *
 * The same loader is stashed on `handle` so preloadRoute() can start the
 * download from hover or focus, before the click.
 */
function page(load: PageLoader): PageRoute {
  return {
    lazy: async () => {
      try {
        return { Component: (await load()).default }
      } catch (error) {
        // A rejected `lazy` never reaches an errorElement — the route never
        // finishes initializing, so React Router leaves the outlet empty and
        // the operator gets a blank page under an otherwise working shell.
        // Resolving to a component that reports the failure keeps the
        // recovery inside the outlet, where the shell still frames it.
        // Render-time crashes stay with the shell's RouteErrorBoundary.
        return { Component: () => <RouteLoadError error={error} /> }
      }
    },
    // Partial hydration: on a cold visit the entered route's module is
    // resolved before its own first render, and this is what the router draws
    // in the meantime. Because it sits on the *child* route rather than the
    // root, the shell above it — top bar, sidebar, breadcrumbs — still paints
    // immediately and only the content area waits. Hanging it on the root
    // instead blanked the whole page for the length of the download.
    hydrateFallbackElement: <Skeleton height={192} className="m-6" />,
    handle: { preload: load } satisfies PreloadHandle,
  }
}

export const routes: RouteObject[] = [
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    path: '/',
    element: (
      <AuthGuard>
        <AppShellLayout />
      </AuthGuard>
    ),
    children: [
      { index: true, ...page(() => import('@/pages/fleet/FleetOverviewPage')) },
      { path: 'health', ...page(() => import('@/pages/health/HealthPage')) },
      { path: 'system/runtime', ...page(() => import('@/pages/system/RuntimePage')) },
      { path: 'namespaces', ...page(() => import('@/pages/namespaces/NamespacesListPage')) },
      { path: 'batch-runs', ...page(() => import('@/pages/batch-runs/BatchRunsListPage')) },
      { path: 'batch-runs/:id', ...page(() => import('@/pages/batch-runs/BatchRunDetailPage')) },
      { path: 'demo-data', ...page(() => import('@/pages/demo-data/DemoDataPage')) },
      { path: 'danger-zone', ...page(() => import('@/pages/danger-zone/DangerZonePage')) },
      {
        path: 'ns/:ns',
        element: <Outlet />,
        children: [
          { index: true, ...page(() => import('@/pages/ns/NamespaceOverviewPage')) },
          { path: 'batch-runs', ...page(() => import('@/pages/batch-runs/BatchRunsListPage')) },
          {
            path: 'batch-runs/:id',
            ...page(() => import('@/pages/batch-runs/BatchRunDetailPage')),
          },
          { path: 'catalog', ...page(() => import('@/pages/ns/catalog/CatalogStatusPage')) },
          { path: 'catalog/items', ...page(() => import('@/pages/ns/catalog/CatalogItemsPage')) },
          {
            path: 'catalog/items/:id',
            ...page(() => import('@/pages/ns/catalog/CatalogItemDetailPage')),
          },
          { path: 'subjects', ...page(() => import('@/pages/ns/subjects/SubjectsListPage')) },
          {
            path: 'subjects/:id',
            ...page(() => import('@/pages/ns/subjects/SubjectInspectorPage')),
          },
          { path: 'events', ...page(() => import('@/pages/ns/events/EventsPage')) },
          { path: 'trending', ...page(() => import('@/pages/ns/trending/TrendingPage')) },
          { path: 'config', ...page(() => import('@/pages/ns/config/NamespaceConfigPage')) },
          // Unmatched sub-paths under a namespace resolve here rather than
          // escaping the shell to React Router's default error screen.
          { path: '*', ...page(() => import('@/pages/not-found/NotFoundPage')) },
        ],
      },
      { path: '*', ...page(() => import('@/pages/not-found/NotFoundPage')) },
    ],
  },
]

/**
 * preloadRoute starts the chunk download for an in-app destination without
 * navigating to it. RouterLink calls this on hover and focus so the module is
 * usually already in the module registry by the time the click lands.
 *
 * This deliberately only warms the *module*. Page data stays with the page's
 * own queries, so pointing at a link never fires an API request and never
 * mutates anything.
 *
 * Failures are swallowed: a preload that loses the network must not surface as
 * an unhandled rejection. The real navigation re-imports the same specifier,
 * and a genuine failure is reported by page()'s catch, which offers a reload —
 * the recovery a stale or missing chunk actually needs.
 */
export function preloadRoute(to: string): void {
  // Only in-app paths are routable; bail on external URLs, mailto:, #anchors.
  if (!to.startsWith('/')) return

  let matches: ReturnType<typeof matchRoutes>
  try {
    matches = matchRoutes(routes, to)
  } catch {
    return
  }

  for (const match of matches ?? []) {
    const handle = match.route.handle as PreloadHandle | undefined
    void handle?.preload?.().catch(() => {})
  }
}
