import { useLocation, useNavigation } from 'react-router-dom'

/**
 * useResolvedPathname returns the path the shell should present as current:
 * the destination of an in-flight navigation if there is one, otherwise the
 * committed location.
 *
 * React Router holds `useLocation()` on the outgoing route until the incoming
 * route's module has loaded. Anything deriving "where am I" from it alone —
 * the sidebar's selected item above all — keeps pointing at the page the
 * operator just left for as long as the download takes. Clicking a
 * destination and watching the previous one stay highlighted reads as a
 * dropped click, which is the complaint this hook exists to answer.
 *
 * Callers that must reflect what is actually rendered (breadcrumbs over the
 * live outlet, a page's own data) should keep using useLocation().
 */
export default function useResolvedPathname(): string {
  const location = useLocation()
  const navigation = useNavigation()
  return navigation.location?.pathname ?? location.pathname
}
