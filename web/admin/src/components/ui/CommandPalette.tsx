import { useEffect, useMemo, useRef, useState } from 'react'
import Modal from './Modal'
import Input from './Input'
import Kbd from './Kbd'

export interface Command {
  id: string
  label: string
  hint?: string // short mono description (e.g. category, path)
  action: () => void
}

// Module-level registry. Pages call useRegisterCommand() to add commands; the
// palette subscribes and renders the current set.
const REGISTRY = new Map<string, Command>()
const SUBSCRIBERS = new Set<() => void>()

function notify() {
  for (const cb of SUBSCRIBERS) cb()
}

// Register a command for the lifetime of the calling component. label can
// change without re-registering; action is held by ref so its body can use
// fresh closures without triggering re-registration churn.
export function useRegisterCommand(id: string, label: string, action: () => void, hint?: string) {
  const actionRef = useRef(action)
  actionRef.current = action

  useEffect(() => {
    REGISTRY.set(id, {
      id,
      label,
      hint,
      action: () => actionRef.current(),
    })
    notify()
    return () => {
      REGISTRY.delete(id)
      notify()
    }
  }, [id, label, hint])
}

function useCommandList(): Command[] {
  const [list, setList] = useState<Command[]>(() => Array.from(REGISTRY.values()))
  useEffect(() => {
    const update = () => setList(Array.from(REGISTRY.values()))
    SUBSCRIBERS.add(update)
    return () => {
      SUBSCRIBERS.delete(update)
    }
  }, [])
  return list
}

// Global palette. Mount once near the app root (e.g. inside AppShell). The
// Cmd+K / Ctrl+K listener is window-wide.
export default function CommandPalette() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const list = useCommandList()

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen((v) => !v)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
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

  const close = () => {
    setOpen(false)
    setQuery('')
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
        onChange={(e) => setQuery(e.target.value)}
        className="w-full mb-3"
      />
      {filtered.length === 0 ? (
        <p className="text-sm text-muted">
          <span className="font-mono">[PEND]</span> no commands match
          {query ? <> for “{query}”</> : <> (registry empty)</>}.
        </p>
      ) : (
        <ul className="flex flex-col gap-0.5">
          {filtered.map((cmd) => (
            <li key={cmd.id}>
              <button
                type="button"
                onClick={() => {
                  cmd.action()
                  close()
                }}
                className="w-full text-left px-2 py-1.5 rounded-sm hover:bg-surface-raised text-sm text-primary flex items-baseline justify-between gap-3"
              >
                <span>{cmd.label}</span>
                {cmd.hint ? (
                  <span className="font-mono text-xs text-muted">{cmd.hint}</span>
                ) : null}
              </button>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  )
}
