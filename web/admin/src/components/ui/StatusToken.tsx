// Bracketed dmesg-style status token. Fixed 6-char width, mono font, color
// carries the semantic. The [ RUN] state pulses via the pulse-run keyframe in
// index.css.
//
// See DESIGN.md §2.5 for the canonical token table.

export type StatusState = 'ok' | 'run' | 'idle' | 'warn' | 'fail' | 'pend'

interface StatusTokenProps {
  state: StatusState
  title?: string // hover tooltip for the verbose state phrase
}

const LABEL: Record<StatusState, string> = {
  ok:   '[ OK ]',
  run:  '[ RUN]',
  idle: '[IDLE]',
  warn: '[WARN]',
  fail: '[FAIL]',
  pend: '[PEND]',
}

const COLOR: Record<StatusState, string> = {
  ok:   'text-success',
  run:  'text-accent animate-pulse-run',
  idle: 'text-muted',
  warn: 'text-warning',
  fail: 'text-danger',
  pend: 'text-muted',
}

export default function StatusToken({ state, title }: StatusTokenProps) {
  return (
    <span
      className={`font-mono whitespace-pre ${COLOR[state]}`}
      title={title}
      aria-label={`status: ${state}`}
    >
      {LABEL[state]}
    </span>
  )
}
