import { useCallback, useSyncExternalStore } from 'react'

/**
 * useMediaQuery reports whether a CSS media query currently matches, and
 * re-renders when that changes.
 *
 * Used where a layout difference has to be structural rather than a CSS
 * `display` toggle — the shell renders the sidebar as a drawer on narrow
 * screens, which means mounting a different element, not hiding one.
 *
 * Built on useSyncExternalStore because that is exactly what a MediaQueryList
 * is: external state React subscribes to. The state-plus-effect version has to
 * re-read the list inside an effect to stay correct across query changes,
 * which costs a cascading render on every mount.
 */
export default function useMediaQuery(query: string): boolean {
  const supported = typeof window !== 'undefined' && typeof window.matchMedia === 'function'

  const subscribe = useCallback(
    (onStoreChange: () => void) => {
      if (!supported) return () => {}
      const list = window.matchMedia(query)
      list.addEventListener('change', onStoreChange)
      return () => list.removeEventListener('change', onStoreChange)
    },
    [query, supported],
  )

  const getSnapshot = useCallback(
    () => (supported ? window.matchMedia(query).matches : false),
    [query, supported],
  )

  // Server snapshot: no DOM means no match, so the shell renders its wide
  // layout — the same thing it does before hydration today.
  return useSyncExternalStore(subscribe, getSnapshot, () => false)
}
