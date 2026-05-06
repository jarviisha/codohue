import type { ReactNode } from 'react'
import Icon from '../Icon'
import Button from './Button'

type NoticeTone = 'accent' | 'success' | 'warning' | 'danger'

interface NoticeProps {
  children: ReactNode
  tone?: NoticeTone
  role?: 'alert' | 'status'
  onDismiss?: () => void
  className?: string
}

const toneClasses: Record<NoticeTone, string> = {
  accent: 'border-accent/20 bg-accent-subtle text-accent',
  success: 'border-success/30 bg-success-bg text-success',
  warning: 'border-warning/30 bg-warning-bg text-warning',
  danger: 'border-danger/25 bg-danger-bg text-danger',
}

export default function Notice({
  children,
  tone = 'accent',
  role,
  onDismiss,
  className = '',
}: NoticeProps) {
  return (
    <div
      role={role}
      className={[
        'flex items-center justify-between rounded-lg border px-4 py-3 text-sm font-medium',
        toneClasses[tone],
        className,
      ].filter(Boolean).join(' ')}
    >
      <div>{children}</div>
      {onDismiss && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onDismiss}
          aria-label="Dismiss notice"
          className="ml-4"
        >
          <Icon name="x" size={14} />
        </Button>
      )}
    </div>
  )
}
