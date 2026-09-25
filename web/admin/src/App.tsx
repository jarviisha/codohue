import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LinkProvider, Theme } from '@astryxdesign/core'
import RouterLink from '@/components/RouterLink'
import { routes } from '@/routes'
import { neutralTheme } from '@astryxdesign/theme-neutral/built'
import { useThemeMode } from '@/services/themeMode'

// Pages are split out of the main bundle so a cold visit only ships the shell
// + the entered route. The route table — and the loader each route preloads
// from — lives in routes.tsx.
const router = createBrowserRouter(routes)

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Default retry off for 4xx — auth failures should bubble immediately.
      // Real services override retry on a per-query basis when appropriate.
      retry: false,
      refetchOnWindowFocus: false,
    },
  },
})

/**
 * App owns the provider stack as well as the router: Theme needs the persisted
 * colour mode, which only a hook can read, and LinkProvider hands every Astryx
 * link (Button href, Breadcrumbs, SideNav) to React Router instead of letting
 * it fall back to a full-page navigation.
 */
export default function App() {
  const { mode } = useThemeMode()
  return (
    <Theme theme={neutralTheme} mode={mode}>
      <LinkProvider component={RouterLink}>
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </LinkProvider>
    </Theme>
  )
}
