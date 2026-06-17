import { useEffect, useMemo, useRef, useState } from 'react'
import { Badge, Inline, SearchInput, Select, Stack } from '@jarviisha/davinci-react-ui'
import type { LogLine } from '@/services/batchRuns'

type LogLineViewerProps = {
  lines: LogLine[]
  /**
   * When true, scroll to the latest line whenever `lines` grows. Defaults to
   * true; the auto-scroll pauses if the user manually scrolls up so reading
   * older lines doesn't fight the live append.
   */
  follow?: boolean
  height?: number
}

const LEVEL_FILTERS = ['all', 'info', 'warn', 'error'] as const
type LevelFilter = (typeof LEVEL_FILTERS)[number]

const LEVEL_BADGE: Record<string, 'success' | 'warning' | 'danger' | 'neutral'> = {
  info: 'neutral',
  warn: 'warning',
  error: 'danger',
}

export default function LogLineViewer({ lines, follow = true, height = 360 }: LogLineViewerProps) {
  const [level, setLevel] = useState<LevelFilter>('all')
  const [query, setQuery] = useState('')
  const [paused, setPaused] = useState(false)
  const scrollerRef = useRef<HTMLDivElement>(null)

  const filtered = useMemo(() => {
    return lines.filter((l) => {
      if (level !== 'all' && l.level !== level) return false
      if (query !== '' && !l.msg.toLowerCase().includes(query.toLowerCase())) return false
      return true
    })
  }, [lines, level, query])

  // Auto-scroll to the bottom on new line append unless the user paused.
  useEffect(() => {
    if (!follow || paused) return
    const el = scrollerRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [filtered.length, follow, paused])

  return (
    <Stack gap="100">
      <Inline gap="100" align="center" justify="between">
        <Inline gap="100" align="center">
          <Select value={level} onChange={(e) => setLevel(e.target.value as LevelFilter)} size="sm">
            {LEVEL_FILTERS.map((l) => (
              <option key={l} value={l}>
                {l}
              </option>
            ))}
          </Select>
          <SearchInput
            size="sm"
            placeholder="Filter messages"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onClear={() => setQuery('')}
          />
        </Inline>
        <Inline gap="100" align="center">
          <span className="text-foreground-subtle text-xs">
            {filtered.length} / {lines.length}
          </span>
          <button
            type="button"
            onClick={() => setPaused((p) => !p)}
            className="text-foreground-subtle text-xs underline"
          >
            {paused ? 'resume autoscroll' : 'pause autoscroll'}
          </button>
        </Inline>
      </Inline>

      <div
        ref={scrollerRef}
        className="bg-surface-sunken border border-default rounded font-mono text-xs overflow-auto davinci-scrollbar"
        style={{ height }}
      >
        {filtered.length === 0 ? (
          <p className="text-foreground-subtle p-3">No log lines match the current filter.</p>
        ) : (
          <ol className="p-2 list-none m-0">
            {filtered.map((l, i) => (
              <li key={`${l.ts}-${i}`} className="flex gap-2 py-0.5 leading-5">
                <span className="text-foreground-subtle shrink-0 w-24 tabular-nums">
                  {tsShort(l.ts)}
                </span>
                <span className="shrink-0">
                  <Badge variant={LEVEL_BADGE[l.level] ?? 'neutral'}>{l.level}</Badge>
                </span>
                <span className="text-foreground whitespace-pre-wrap wrap-break-word">{l.msg}</span>
              </li>
            ))}
          </ol>
        )}
      </div>
    </Stack>
  )
}

// tsShort renders ISO timestamps as HH:MM:SS.mmm, dropping the date so each
// row stays compact. Returns the raw value when parsing fails (defensive).
function tsShort(raw: string): string {
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return raw
  const pad = (n: number, w = 2) => String(n).padStart(w, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`
}
