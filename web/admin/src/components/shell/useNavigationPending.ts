import { useEffect, useState } from 'react'
import { useNavigation } from 'react-router-dom'

/**
 * How long a navigation may run before the progress bar appears. A destination
 * whose chunk was already warmed by preloadRoute() resolves well inside this,
 * so the common case stays still instead of flickering a bar on every click.
 * Slower loads — a cold chunk, a bad network — cross it and get reported.
 *
 * This gates only the *visible* bar. aria-busy on the outlet and the sidebar's
 * selected destination both move immediately, so nothing is silently
 * unacknowledged during the first 150ms either.
 */
const SHOW_AFTER_MS = 150

export type NavigationPending = {
  /** A route navigation is in flight. True from the click. */
  isPending: boolean
  /** That navigation has run past SHOW_AFTER_MS and is worth drawing. */
  isSlow: boolean
  /** Pathname being navigated to, or null when idle. */
  to: string | null
}

/**
 * useNavigationPending reports whether a route navigation is in flight and
 * whether it has been in flight long enough to draw a progress bar.
 *
 * React Router reports `state === 'loading'` for as long as the destination's
 * module is downloading (see routes.tsx), which is exactly the window where
 * the shell would otherwise sit on the outgoing page with no sign that a click
 * had registered.
 *
 * The slow flag is stored *per navigation* rather than reset when one ends:
 * settling it back to false in an effect would be a synchronous setState in an
 * effect body, which cascades a render on every completed navigation. Keying
 * it to the navigation instead lets it fall out of scope on its own — a stale
 * key simply stops matching.
 */
export default function useNavigationPending(): NavigationPending {
  const navigation = useNavigation()
  const isPending = navigation.state !== 'idle'
  const to = navigation.location?.pathname ?? null
  // React Router stamps each history entry with a unique key, so two
  // successive navigations to the same path are still distinguishable.
  const key = navigation.location?.key ?? to
  const [slowKey, setSlowKey] = useState<string | null>(null)

  useEffect(() => {
    if (!isPending || !key) return
    const timer = window.setTimeout(() => setSlowKey(key), SHOW_AFTER_MS)
    return () => window.clearTimeout(timer)
  }, [isPending, key])

  return { isPending, isSlow: isPending && slowKey != null && slowKey === key, to }
}
