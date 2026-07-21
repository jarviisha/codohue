import { useSyncExternalStore } from 'react'

const STORAGE_KEY = 'codohue-admin-recent-namespaces'
const MAX = 5

// External-store plumbing so useRecentNamespaces() re-renders any subscriber
// when recordRecentNamespace fires from elsewhere in the tree (e.g. the route
// effect in AppShellLayout that records every namespace visit). localStorage
// `storage` events only fire across tabs — same-tab updates need our own
// notify channel.
const subscribers = new Set<() => void>()
function notify() {
  for (const fn of subscribers) fn()
}

function read(): string[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((v): v is string => typeof v === 'string').slice(0, MAX)
  } catch {
    return []
  }
}

let snapshot: string[] = read()

function write(next: string[]) {
  snapshot = next.slice(0, MAX)
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(snapshot))
  } catch {
    // localStorage may throw in private-mode Safari etc. — keep the in-memory
    // snapshot so the UI still updates, even if it won't survive reload.
  }
  notify()
}

/**
 * Record a namespace visit. The most recently visited namespace ends up at
 * the head of the list, deduped, capped at MAX. Safe to call from a render
 * effect on every route change — no-ops when the namespace already leads.
 */
export function recordRecentNamespace(namespace: string) {
  if (!namespace) return
  if (snapshot[0] === namespace) return
  const next = [namespace, ...snapshot.filter((n) => n !== namespace)]
  write(next)
}

/**
 * useRecentNamespaces returns the recent-namespace list, ordered newest first.
 * Re-renders subscribers when recordRecentNamespace runs anywhere in the tree.
 */
export function useRecentNamespaces(): string[] {
  return useSyncExternalStore(
    (cb) => {
      subscribers.add(cb)
      return () => {
        subscribers.delete(cb)
      }
    },
    () => snapshot,
    () => [],
  )
}
