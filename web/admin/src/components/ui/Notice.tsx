import type { ReactNode } from 'react'
import StatusToken, { type StatusState } from './StatusToken'

type NoticeTone = 'ok' | 'warn' | 'fail' | 'info'

interface NoticeProps {
  tone?: NoticeTone
  title?: ReactNode
  onDismiss?: () => void
  children: ReactNode
}

const BORDER: Record<NoticeTone, string> = {
  ok:   'border-l-success',
  warn: 'border-l-warning',
  fail: 'border-l-danger',
  info: 'border-l-accent',
}

const TOKEN_STATE: Record<NoticeTone, StatusState | null> = {
  ok:   'ok',
  warn: 'warn',
  fail: 'fail',
  info: null,  // info notice skips the [TOKEN] prefix
}

// 4px left border + status text + no bg fill. Terminal/Unix-DNA pattern from
// DESIGN.md §6.1. Body text uses text-primary; the tone signal lives in the
// border + the optional StatusToken prefix.
export default function Notice({ tone = 'info', title, onDismiss, children }: NoticeProps) {
  const tokenState = TOKEN_STATE[tone]
  return (
    <aside
      role={tone === 'fail' ? 'alert' : 'status'}
      className={`border-l-4 ${BORDER[tone]} bg-transparent pl-4 pr-3 py-3 flex items-start gap-3`}
    >
      <div className="flex-1 min-w-0">
        {(tokenState || title) && (
          <div className="flex items-center gap-2 mb-1">
            {tokenState ? <StatusToken state={tokenState} /> : null}
            {title ? <span className="text-sm font-semibold text-primary">{title}</span> : null}
          </div>
        )}
        <div className="text-sm text-primary">{children}</div>
      </div>
      {onDismiss ? (
        <button
          type="button"
          onClick={onDismiss}
          className="text-muted hover:text-primary h-6 w-6 flex items-center justify-center rounded-sm font-mono"
          aria-label="Dismiss notice"
        >
          ×
        </button>
      ) : null}
    </aside>
  )
}
