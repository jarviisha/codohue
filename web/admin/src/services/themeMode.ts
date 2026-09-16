import { useEffect, useState, useSyncExternalStore } from 'react'

/**
 * Astryx's <Theme> takes the colour mode as a prop and syncs `data-theme` onto
 * <html> — it does not persist the choice. This module owns that bit.
 *
 * The preference lives in a module-level store rather than a context because
 * two unrelated places read it: the <Theme> at the app root and the theme menu
 * in the top bar. A store keeps them in sync without threading a provider
 * through the tree.
 *
 * The storage key matches the inlined no-flash script in index.html; changing
 * one means changing the other.
 */
export type ThemeMode = 'light' | 'dark' | 'system'

export const THEME_STORAGE_KEY = 'codohue-admin-theme'

function readStored(): ThemeMode {
  try {
    const raw = localStorage.getItem(THEME_STORAGE_KEY)
    if (raw === 'light' || raw === 'dark' || raw === 'system') return raw
  } catch {
    // Private mode / blocked storage — fall through to the system default.
  }
  return 'system'
}

let mode: ThemeMode = readStored()
const listeners = new Set<() => void>()

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function setThemeMode(next: ThemeMode) {
  if (next === mode) return
  mode = next
  try {
    localStorage.setItem(THEME_STORAGE_KEY, next)
  } catch {
    // Preference is best-effort; the in-memory mode still applies.
  }
  listeners.forEach((l) => l())
}

function prefersDark(): boolean {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

export function useThemeMode() {
  const current = useSyncExternalStore(
    subscribe,
    () => mode,
    () => 'system' as ThemeMode,
  )
  const [systemDark, setSystemDark] = useState(prefersDark)

  useEffect(() => {
    const query = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => setSystemDark(query.matches)
    query.addEventListener('change', onChange)
    return () => query.removeEventListener('change', onChange)
  }, [])

  const resolvedMode: 'light' | 'dark' =
    current === 'system' ? (systemDark ? 'dark' : 'light') : current

  return { mode: current, resolvedMode, setMode: setThemeMode }
}
