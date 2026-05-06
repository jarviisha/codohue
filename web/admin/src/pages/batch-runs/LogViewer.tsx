import type { LogEntry } from '../../types'

export default function LogViewer({ entries }: { entries: LogEntry[] }) {
  if (entries.length === 0) {
    return (
      <p className="text-[11px] text-muted italic px-3 py-2">No log entries captured.</p>
    )
  }
  return (
    <div className="font-mono text-[11px] leading-5 max-h-56 overflow-y-auto">
      {entries.map((e, i) => {
        const color =
          e.level === 'error' ? 'text-danger' :
          e.level === 'warn'  ? 'text-warning' :
          'text-secondary'
        const levelTag =
          e.level === 'error' ? 'ERR' :
          e.level === 'warn'  ? 'WRN' :
          'INF'
        const ts = e.ts.slice(11, 23)
        return (
          <div key={i} className={`flex gap-2 px-3 py-0.5 hover:bg-surface-raised ${color}`}>
            <span className="text-muted shrink-0">{ts}</span>
            <span className="shrink-0 w-7">{levelTag}</span>
            <span className="break-all">{e.msg}</span>
          </div>
        )
      })}
    </div>
  )
}
