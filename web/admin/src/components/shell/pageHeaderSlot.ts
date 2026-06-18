import { createContext } from 'react'

/**
 * pageHeaderSlot is the DOM element inside AppShellHeader that <PageHeader>
 * teleports its children into. AppShellLayout exposes the slot through this
 * context via a ref callback so portal targets stay React-aware (no manual
 * document.getElementById, no first-render flash of empty header).
 *
 * Null on initial render until the slot mounts; PageHeader renders nothing
 * until the value is set.
 */
export const PageHeaderSlotContext = createContext<HTMLElement | null>(null)
