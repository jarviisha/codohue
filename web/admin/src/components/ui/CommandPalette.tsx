import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import Modal from './Modal'
import Input from './Input'
import Kbd from './Kbd'
import { useCommandList } from './commandRegistry'

// Custom event that lets non-keyboard surfaces (e.g. the TopBar Cmd+K
// button) toggle the palette without lifting state up. Listened to by the
// effect below alongside the global Cmd+K shortcut.
export const COMMAND_PALETTE_TOGGLE_EVENT = 'codohue:toggle-command-palette'

// Global palette. Mount once near the app root (e.g. inside AppShell). The
// Cmd+K / Ctrl+K listener is window-wide; clicking the TopBar Cmd+K button
// dispatches the same toggle via COMMAND_PALETTE_TOGGLE_EVENT.
//
// Keyboard flow inside the palette:
//   ↑ / ↓     — move highlight (wraps)
//   Enter     — run the highlighted command (or the only filtered item)
//   Escape    — handled by the underlying Modal
//   any other — Input swallows for filter typing
export default function CommandPalette() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [activeIdx, setActiveIdx] = useState(0)
  const list = useCommandList()
  const listRef = useRef<HTMLUListElement>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent | globalThis.KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen((v) => !v)
      }
    }
    const onToggle = () => setOpen((v) => !v)
    window.addEventListener('keydown', onKey as EventListener)
    window.addEventListener(COMMAND_PALETTE_TOGGLE_EVENT, onToggle)
    return () => {
      window.removeEventListener('keydown', onKey as EventListener)
      window.removeEventListener(COMMAND_PALETTE_TOGGLE_EVENT, onToggle)
    }
  }, [])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return list
    return list.filter(
      (c) =>
        c.label.toLowerCase().includes(q) ||
        (c.hint ?? '').toLowerCase().includes(q),
    )
  }, [list, query])

  // Clamp the highlight on render so the index stays in range even when the
  // filtered set shrinks (e.g. typing into the filter narrows the match
  // count). Avoids a setState-in-effect lint hit and keeps the reset path
  // explicit on the input change handler below.
  const safeActiveIdx =
    filtered.length === 0 ? 0 : Math.min(activeIdx, filtered.length - 1)

  // Keep the active item in view as the operator arrows through a long list.
  useEffect(() => {
    if (!open) return
    const item = listRef.current?.children[safeActiveIdx] as HTMLElement | undefined
    item?.scrollIntoView({ block: 'nearest' })
  }, [safeActiveIdx, open])

  const close = () => {
    setOpen(false)
    setQuery('')
    setActiveIdx(0)
  }

  const onQueryChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setQuery(e.target.value)
    setActiveIdx(0)
  }

  const runActive = () => {
    const cmd = filtered[safeActiveIdx]
    if (!cmd) return
    cmd.action()
    close()
  }

  const onInputKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIdx((i) => (filtered.length === 0 ? 0 : (i + 1) % filtered.length))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIdx((i) =>
        filtered.length === 0 ? 0 : (i - 1 + filtered.length) % filtered.length,
      )
    } else if (e.key === 'Enter') {
      e.preventDefault()
      runActive()
    }
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title={
        <span className="flex items-center gap-2">
          Command palette <Kbd>Cmd+K</Kbd>
        </span>
      }
      size="md"
    >
      <Input
        autoFocus
        placeholder="Type a command…"
        value={query}
        onChange={onQueryChange}
        onKeyDown={onInputKey}
        aria-controls="command-palette-list"
        aria-activedescendant={
          filtered[safeActiveIdx]
            ? `command-palette-item-${filtered[safeActiveIdx].id}`
            : undefined
        }
        className="w-full mb-3"
      />
      {filtered.length === 0 ? (
        <p className="text-sm text-muted">
          <span className="font-mono text-xs uppercase tracking-[0.04em]">PEND</span>{' '}
          no commands match
          {query ? <> for "{query}"</> : <> (registry empty)</>}.
        </p>
      ) : (
        <ul
          ref={listRef}
          id="command-palette-list"
          role="listbox"
          className="flex flex-col gap-0.5"
        >
          {filtered.map((cmd, idx) => {
            const active = idx === safeActiveIdx
            return (
              <li key={cmd.id}>
                <button
                  type="button"
                  id={`command-palette-item-${cmd.id}`}
                  role="option"
                  aria-selected={active}
                  onClick={() => {
                    cmd.action()
                    close()
                  }}
                  onMouseEnter={() => setActiveIdx(idx)}
                  className={[
                    'w-full text-left px-2 py-1.5 rounded-sm text-sm flex items-baseline justify-between gap-3',
                    active
                      ? 'bg-surface-raised text-primary'
                      : 'text-primary hover:bg-surface-raised',
                  ].join(' ')}
                >
                  <span>{cmd.label}</span>
                  {cmd.hint ? (
                    <span className="font-mono text-xs text-muted">{cmd.hint}</span>
                  ) : null}
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </Modal>
  )
}
