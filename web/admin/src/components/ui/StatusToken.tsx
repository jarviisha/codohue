// Compact status token. Text and background tint carry the semantic state
// while preserving a fixed-width mono shape for table scanning. The RUN
// state pulses via the pulse-run keyframe in index.css.
//
// See DESIGN.md §2.5 for the canonical token table.

export type StatusState = 'ok' | 'run' | 'idle' | 'warn' | 'fail' | 'pend'

interface StatusTokenProps {
  state: StatusState
  title?: string // hover tooltip for the verbose state phrase
}

const LABEL: Record<StatusState, string> = {
  ok:   'OK',
  run:  'RUN',
  idle: 'IDLE',
  warn: 'WARN',
  fail: 'FAIL',
  pend: 'PEND',
}

const STYLE: Record<StatusState, string> = {
  ok:   'bg-success/20 text-success',
  run:  'bg-accent/20 text-accent animate-pulse-run',
  idle: 'bg-surface-raised text-secondary',
  warn: 'bg-warning/20 text-warning',
  fail: 'bg-danger/20 text-danger',
  pend: 'bg-surface-raised text-secondary',
}

export default function StatusToken({ state, title }: StatusTokenProps) {
  return (
    <span
      className={`inline-flex h-5 min-w-10 items-center justify-center rounded-sm px-1.5 font-mono text-[11px] font-semibold uppercase leading-none tracking-[0.04em] ${STYLE[state]}`}
      title={title}
      aria-label={`status: ${state}`}
    >
      {LABEL[state]}
    </span>
  )
}
