import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  Dialog,
  DialogContent,
  Input,
  Stack,
} from '@jarviisha/davinci-react-ui'
import { useRecentNamespaces } from '@/services/recentNamespaces'

type Command = {
  /** Stable key used for React + selection memory. */
  id: string
  /** First line — the user-visible label. */
  title: string
  /** Second line — destination path or extra context. */
  subtitle?: string
  /** Hidden search terms in addition to title. */
  keywords?: string
  /** Group label that this command sits under. */
  group: string
  run: () => void
}

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * CommandPalette is the Cmd+K / Ctrl+K nav. It indexes every static route
 * plus contextual entries (current namespace's sub-pages, recent namespaces,
 * deep-link jumps like `#123` to a batch run or `subject:user-42` to the
 * inspector) so keyboard-driven operators don't need the sidebar.
 *
 * The palette renders only while `open` is true so each opening starts with
 * a fresh query + selection — no useEffect-driven reset needed.
 */
export default function CommandPalette({ open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} size="md">
      {open && <Palette onClose={() => onOpenChange(false)} />}
    </Dialog>
  )
}

function Palette({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const location = useLocation()
  const recents = useRecentNamespaces()
  // Colocate query + selected in one state so editing the query also resets
  // the highlighted row in the same update — avoids the React 19
  // set-state-in-effect rule that a separate sync effect would trip.
  const [{ query, selected }, setState] = useState<{ query: string; selected: number }>({
    query: '',
    selected: 0,
  })
  const setQuery = (q: string) => setState({ query: q, selected: 0 })
  const setSelected = (next: number | ((cur: number) => number)) =>
    setState((s) => ({
      ...s,
      selected: typeof next === 'function' ? next(s.selected) : next,
    }))
  const listRef = useRef<HTMLDivElement | null>(null)

  // Derive the current namespace from the URL so the palette can include
  // contextual subpages without an extra hook.
  const currentNs = useMemo(() => {
    const m = location.pathname.match(/^\/ns\/([^/]+)/)
    return m ? decodeURIComponent(m[1]) : null
  }, [location.pathname])

  const commands = useMemo(
    () => buildCommands({ navigate, onClose, currentNs, recents }),
    [navigate, onClose, currentNs, recents],
  )

  // Deep-link parsers — typing `#42` jumps to batch-runs/42, `user-42` jumps
  // to the subject inspector when a namespace is active. Synthetic entries
  // appear at the head of the filtered list so Enter does the right thing.
  const deepLinks = useMemo<Command[]>(() => {
    const q = query.trim()
    if (q === '') return []
    const out: Command[] = []
    const runId = q.match(/^#?(\d+)$/)
    if (runId) {
      const id = runId[1]
      out.push({
        id: `deeplink-run-${id}`,
        title: `Open batch run #${id}`,
        subtitle: `/batch-runs/${id}`,
        group: 'Jump to',
        run: () => {
          navigate(`/batch-runs/${id}`)
          onClose()
        },
      })
    }
    if (currentNs && q.length >= 2 && !runId) {
      out.push({
        id: `deeplink-subject-${q}`,
        title: `Open subject "${q}"`,
        subtitle: `/ns/${currentNs}/subjects/${q}`,
        group: 'Jump to',
        run: () => {
          navigate(`/ns/${encodeURIComponent(currentNs)}/subjects/${encodeURIComponent(q)}`)
          onClose()
        },
      })
    }
    return out
  }, [query, currentNs, navigate, onClose])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    const all = [...deepLinks, ...commands]
    if (q === '') return all
    return all.filter((c) => {
      const haystack = `${c.title} ${c.subtitle ?? ''} ${c.keywords ?? ''} ${c.group}`.toLowerCase()
      return haystack.includes(q)
    })
  }, [commands, deepLinks, query])

  // Clamp the selected index against the current filtered length so a
  // shrinking list (user typed more chars) never points past the end. Derived
  // in render instead of synced via an effect to satisfy React 19's
  // set-state-in-effect rule.
  const selectedClamped = filtered.length === 0 ? 0 : Math.min(selected, filtered.length - 1)

  // Keep the highlighted row in view when the user arrows down the list.
  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(
      `[data-cmd-index="${selectedClamped}"]`,
    )
    el?.scrollIntoView({ block: 'nearest' })
  }, [selectedClamped])

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelected(
        filtered.length === 0 ? 0 : Math.min(selectedClamped + 1, filtered.length - 1),
      )
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelected(Math.max(selectedClamped - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const cmd = filtered[selectedClamped]
      if (cmd) cmd.run()
    }
  }

  // Group commands by their `group` field, preserving the order they appear
  // in the filtered list so deep-link suggestions stay at the top.
  const grouped = useMemo(() => {
    const seen: string[] = []
    const byGroup = new Map<string, Command[]>()
    for (const c of filtered) {
      if (!byGroup.has(c.group)) {
        byGroup.set(c.group, [])
        seen.push(c.group)
      }
      byGroup.get(c.group)!.push(c)
    }
    return seen.map((g) => ({ group: g, items: byGroup.get(g)! }))
  }, [filtered])

  return (
    <DialogContent>
      <Stack onKeyDown={onKeyDown}>
        <Input
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Jump to… (try a page, namespace, or #run-id)"
          aria-label="Command palette search"
        />
        <div
          ref={listRef}
          className="max-h-96 overflow-y-auto"
          role="listbox"
          aria-label="Commands"
        >
          {filtered.length === 0 ? (
            <p className="text-foreground-subtle text-sm px-2 py-4 text-center">
              No matches.
            </p>
          ) : (
            <Stack>
              {grouped.map(({ group, items }) => (
                <Stack key={group}>
                  <span className="text-foreground-subtle text-xs uppercase tracking-wide px-2">
                    {group}
                  </span>
                  {items.map((cmd) => {
                    const absoluteIndex = filtered.indexOf(cmd)
                    const isSelected = absoluteIndex === selectedClamped
                    return (
                      <button
                        key={cmd.id}
                        type="button"
                        role="option"
                        aria-selected={isSelected}
                        data-cmd-index={absoluteIndex}
                        onClick={() => cmd.run()}
                        onMouseEnter={() => setSelected(absoluteIndex)}
                        className={[
                          'text-left rounded px-2 py-2 transition-colors',
                          'focus:outline-none',
                          isSelected ? 'bg-surface-sunken' : 'hover:bg-surface-sunken',
                        ].join(' ')}
                      >
                        <span className="text-foreground text-sm font-medium block">
                          {cmd.title}
                        </span>
                        {cmd.subtitle && (
                          <span className="text-foreground-subtle text-xs block">
                            {cmd.subtitle}
                          </span>
                        )}
                      </button>
                    )
                  })}
                </Stack>
              ))}
            </Stack>
          )}
        </div>
      </Stack>
    </DialogContent>
  )
}

function buildCommands({
  navigate,
  onClose,
  currentNs,
  recents,
}: {
  navigate: ReturnType<typeof useNavigate>
  onClose: () => void
  currentNs: string | null
  recents: string[]
}): Command[] {
  const go = (path: string) => () => {
    navigate(path)
    onClose()
  }

  const commands: Command[] = []

  // Global nav. Stable order — the operator's muscle memory should hold
  // regardless of which page they're on.
  commands.push(
    { id: 'fleet', title: 'Fleet', subtitle: '/', group: 'Global', run: go('/'), keywords: 'overview home' },
    { id: 'namespaces', title: 'Namespaces', subtitle: '/namespaces', group: 'Global', run: go('/namespaces') },
    { id: 'namespaces-new', title: 'New namespace', subtitle: '/namespaces', group: 'Global', run: go('/namespaces?new=1'), keywords: 'create' },
    { id: 'batch-runs', title: 'Batch runs', subtitle: '/batch-runs', group: 'Global', run: go('/batch-runs') },
    { id: 'health', title: 'Health', subtitle: '/health', group: 'Global', run: go('/health') },
    { id: 'demo-data', title: 'Demo data', subtitle: '/demo-data', group: 'Global', run: go('/demo-data'), keywords: 'seed sample bundled' },
    { id: 'danger-zone', title: 'Danger zone', subtitle: '/danger-zone', group: 'Global', run: go('/danger-zone'), keywords: 'reset wipe' },
  )

  // Namespace-scoped commands appear only when a namespace is active.
  if (currentNs) {
    const ns = encodeURIComponent(currentNs)
    commands.push(
      { id: `ns-overview`, title: `Overview · #${currentNs}`, subtitle: `/ns/${currentNs}`, group: 'Namespace', run: go(`/ns/${ns}`) },
      { id: `ns-batch-runs`, title: `Batch runs · #${currentNs}`, subtitle: `/ns/${currentNs}/batch-runs`, group: 'Namespace', run: go(`/ns/${ns}/batch-runs`) },
      { id: `ns-catalog`, title: `Catalog · #${currentNs}`, subtitle: `/ns/${currentNs}/catalog`, group: 'Namespace', run: go(`/ns/${ns}/catalog`) },
      { id: `ns-catalog-items`, title: `Catalog items · #${currentNs}`, subtitle: `/ns/${currentNs}/catalog/items`, group: 'Namespace', run: go(`/ns/${ns}/catalog/items`) },
      { id: `ns-subjects`, title: `Subjects · #${currentNs}`, subtitle: `/ns/${currentNs}/subjects`, group: 'Namespace', run: go(`/ns/${ns}/subjects`), keywords: 'inspector recommend' },
      { id: `ns-events`, title: `Events · #${currentNs}`, subtitle: `/ns/${currentNs}/events`, group: 'Namespace', run: go(`/ns/${ns}/events`), keywords: 'tail ingest' },
      { id: `ns-trending`, title: `Trending · #${currentNs}`, subtitle: `/ns/${currentNs}/trending`, group: 'Namespace', run: go(`/ns/${ns}/trending`) },
    )
  }

  // Recent namespaces (excluding the current one) — one-click jump to a
  // sibling without leaving the keyboard.
  for (const ns of recents) {
    if (ns === currentNs) continue
    commands.push({
      id: `recent-${ns}`,
      title: `$${ns}`,
      subtitle: `Jump to /ns/${ns}`,
      group: 'Recent namespaces',
      run: go(`/ns/${encodeURIComponent(ns)}`),
    })
  }

  return commands
}
