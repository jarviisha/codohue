import { useEffect, useMemo, useState } from 'react'
import Modal from './Modal'
import Input from './Input'
import Kbd from './Kbd'
import { useCommandList } from './commandRegistry'

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
          {query ? <> for "{query}"</> : <> (registry empty)</>}.
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
