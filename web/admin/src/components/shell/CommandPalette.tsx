import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { CommandPalette as AstryxCommandPalette, Stack, Text } from '@astryxdesign/core'
import type { SearchableItem, SearchSource } from '@astryxdesign/core'
import { useRecentNamespaces } from '@/services/recentNamespaces'
import useNamespaceParam from '@/components/shell/useNamespaceParam'

/**
 * A palette entry. `label` is what the operator reads and types against;
 * everything else rides in auxiliaryData so Astryx's search can stay on the
 * label + keywords it already understands.
 */
type Command = SearchableItem<{
  /** Second line — destination path or extra context. */
  subtitle?: string
  /** Hidden search terms in addition to the label. */
  keywords?: string
  /** Group this command belongs to, shown alongside the subtitle. */
  group: string
  run: () => void
}>

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
 * Filtering, keyboard navigation and the empty state come from Astryx; this
 * file only builds the command list and runs the picked entry. Deep links are
 * why the search source is hand-written rather than `createStaticSource`:
 * `#42` has to synthesise an entry from the query itself, not match one.
 */
export default function CommandPalette({ open, onOpenChange }: Props) {
  const navigate = useNavigate()
  const recents = useRecentNamespaces()
  // The current namespace decides which contextual subpages the palette offers.
  const currentNs = useNamespaceParam()

  const onClose = () => onOpenChange(false)

  const commands = useMemo(
    () => buildCommands({ navigate, onClose, currentNs, recents }),
    // onClose is recreated every render; onOpenChange is the stable input.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [navigate, onOpenChange, currentNs, recents],
  )

  const searchSource = useMemo<SearchSource<Command>>(
    () => ({
      bootstrap: () => commands,
      search: (query) => {
        const q = query.trim()
        const matches = commands.filter((c) => {
          const aux = c.auxiliaryData!
          const haystack =
            `${c.label} ${aux.subtitle ?? ''} ${aux.keywords ?? ''} ${aux.group}`.toLowerCase()
          return haystack.includes(q.toLowerCase())
        })
        return [...deepLinks(q, currentNs, navigate, onClose), ...matches]
      },
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [commands, currentNs, navigate, onOpenChange],
  )

  return (
    <AstryxCommandPalette<Command>
      isOpen={open}
      onOpenChange={onOpenChange}
      searchSource={searchSource}
      onValueChange={(id) => {
        const hit = commands.find((c) => c.id === id)
        if (hit) hit.auxiliaryData!.run()
      }}
      emptySearchText="No matches."
      renderItem={(item) => (
        <Stack gap={0.5}>
          <Text weight="medium">{item.label}</Text>
          <Text type="supporting" size="xsm">
            {[item.auxiliaryData?.group, item.auxiliaryData?.subtitle]
              .filter(Boolean)
              .join(' · ')}
          </Text>
        </Stack>
      )}
    />
  )
}

/**
 * Deep-link parsers — typing `#42` jumps to batch-runs/42, `user-42` jumps to
 * the subject inspector when a namespace is active. These are synthesised per
 * query and lead the result list so Enter does the right thing.
 */
function deepLinks(
  query: string,
  currentNs: string | null,
  navigate: (to: string) => void,
  onClose: () => void,
): Command[] {
  if (query === '') return []
  const out: Command[] = []
  const runId = query.match(/^#?(\d+)$/)
  if (runId) {
    const id = runId[1]
    out.push({
      id: `deeplink-run-${id}`,
      label: `Open batch run #${id}`,
      auxiliaryData: {
        subtitle: `/batch-runs/${id}`,
        group: 'Jump to',
        run: () => {
          navigate(`/batch-runs/${id}`)
          onClose()
        },
      },
    })
  }
  if (currentNs && query.length >= 2 && !runId) {
    out.push({
      id: `deeplink-subject-${query}`,
      label: `Open subject "${query}"`,
      auxiliaryData: {
        subtitle: `/ns/${currentNs}/subjects/${query}`,
        group: 'Jump to',
        run: () => {
          navigate(`/ns/${encodeURIComponent(currentNs)}/subjects/${encodeURIComponent(query)}`)
          onClose()
        },
      },
    })
  }
  return out
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
  const cmd = (
    id: string,
    label: string,
    group: string,
    subtitle: string,
    run: () => void,
    keywords?: string,
  ): Command => ({ id, label, auxiliaryData: { group, subtitle, run, keywords } })

  const commands: Command[] = []

  // Global nav. Stable order — the operator's muscle memory should hold
  // regardless of which page they're on.
  commands.push(
    cmd('fleet', 'Fleet', 'Global', '/', go('/'), 'overview home'),
    cmd('namespaces', 'Namespaces', 'Global', '/namespaces', go('/namespaces')),
    cmd('namespaces-new', 'New namespace', 'Global', '/namespaces', go('/namespaces?new=1'), 'create'),
    cmd('batch-runs', 'Batch runs', 'Global', '/batch-runs', go('/batch-runs')),
    cmd('health', 'Health', 'Global', '/health', go('/health')),
    cmd('demo-data', 'Demo data', 'Global', '/demo-data', go('/demo-data'), 'seed sample bundled'),
    cmd('danger-zone', 'Danger zone', 'Global', '/danger-zone', go('/danger-zone'), 'reset wipe'),
  )

  // Namespace-scoped commands appear only when a namespace is active.
  if (currentNs) {
    const ns = encodeURIComponent(currentNs)
    commands.push(
      cmd('ns-overview', 'Overview', 'Namespace', `/ns/${currentNs}`, go(`/ns/${ns}`)),
      cmd('ns-batch-runs', 'Batch runs', 'Namespace', `/ns/${currentNs}/batch-runs`, go(`/ns/${ns}/batch-runs`)),
      cmd('ns-catalog', 'Catalog', 'Namespace', `/ns/${currentNs}/catalog`, go(`/ns/${ns}/catalog`)),
      cmd('ns-catalog-items', 'Catalog items', 'Namespace', `/ns/${currentNs}/catalog/items`, go(`/ns/${ns}/catalog/items`)),
      cmd('ns-subjects', 'Subjects', 'Namespace', `/ns/${currentNs}/subjects`, go(`/ns/${ns}/subjects`), 'inspector recommend'),
      cmd('ns-events', 'Events', 'Namespace', `/ns/${currentNs}/events`, go(`/ns/${ns}/events`), 'tail ingest'),
      cmd('ns-trending', 'Trending', 'Namespace', `/ns/${currentNs}/trending`, go(`/ns/${ns}/trending`)),
    )
  }

  // Recent namespaces (excluding the current one) — one-click jump to a
  // sibling without leaving the keyboard.
  for (const ns of recents) {
    if (ns === currentNs) continue
    commands.push(
      cmd(
        `recent-${ns}`,
        `$${ns}`,
        'Recent namespaces',
        `Jump to /ns/${ns}`,
        go(`/ns/${encodeURIComponent(ns)}`),
      ),
    )
  }

  return commands
}
