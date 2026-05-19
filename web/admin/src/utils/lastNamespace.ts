// Remembered "last namespace" the operator was viewing. Lets the sidebar
// keep the per-namespace nav visible after the operator clicks a global
// entry (Demo Data, Danger Zone, Namespaces) so getting back to where they
// were is one click. sessionStorage scopes the memory to a single tab —
// closing the tab resets the context, which matches operator expectations.
//
// The URL stays the single source of truth for "which namespace am I
// actively in"; this helper only powers the soft fallback while no
// namespace is in the URL.

const KEY = 'codohue:last-ns'

export function getLastNamespace(): string | null {
  try {
    return sessionStorage.getItem(KEY)
  } catch {
    return null
  }
}

export function setLastNamespace(name: string): void {
  try {
    sessionStorage.setItem(KEY, name)
  } catch {
    // sessionStorage may be unavailable in private modes — silently degrade.
  }
}

export function clearLastNamespace(): void {
  try {
    sessionStorage.removeItem(KEY)
  } catch {
    // see setLastNamespace.
  }
}
