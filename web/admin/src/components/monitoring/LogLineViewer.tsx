import { useEffect, useMemo, useRef, useState } from 'react'
import { Selector, Stack, TextInput, Token } from '@astryxdesign/core'
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

const LEVEL_TOKEN_COLOR: Record<string, 'gray' | 'orange' | 'red'> = {
  info: 'gray',
  warn: 'orange',
  error: 'red',
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
    <Stack gap={6}>
      <Stack direction="horizontal" gap={4} align="center" justify="between">
        <Stack direction="horizontal" gap={2} align="center">
          <Selector
            size="sm"
            label="Log level"
            isLabelHidden
            value={level}
            onChange={(next) => setLevel(next as LevelFilter)}
            options={[...LEVEL_FILTERS]}
          />
          <TextInput
            size="sm"
            label="Filter messages"
            isLabelHidden
            placeholder="Filter messages"
            value={query}
            onChange={setQuery}
            hasClear
          />
        </Stack>
        <Stack direction="horizontal" gap={2} align="center">
          <span className="text-secondary text-xs">
            {filtered.length} / {lines.length}
          </span>
          <button
            type="button"
            onClick={() => setPaused((p) => !p)}
            className="text-secondary text-xs underline"
          >
            {paused ? 'resume autoscroll' : 'pause autoscroll'}
          </button>
        </Stack>
      </Stack>

      <div
        ref={scrollerRef}
        className="bg-muted border border-border rounded font-mono text-xs overflow-auto"
        style={{ height }}
      >
        {filtered.length === 0 ? (
          <p className="text-secondary p-3">No log lines match the current filter.</p>
        ) : (
          <ol className="p-2 list-none m-0">
            {filtered.map((l, i) => (
              <li key={`${l.ts}-${i}`} className="flex gap-2 py-0.5 leading-5">
                <span className="text-secondary shrink-0 w-24 tabular-nums">
                  {tsShort(l.ts)}
                </span>
                <span className="shrink-0">
                  <Token size="sm" color={LEVEL_TOKEN_COLOR[l.level] ?? 'gray'} label={l.level} />
                </span>
                <span className="text-primary whitespace-pre-wrap wrap-break-word">{l.msg}</span>
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
