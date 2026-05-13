import type { ReactNode } from 'react'

type BadgeTone = 'neutral' | 'accent' | 'success' | 'warning' | 'danger'
type BadgeSize = 'sm' | 'md'

interface BadgeProps {
  children: ReactNode
  tone?: BadgeTone
  size?: BadgeSize
  dot?: boolean
  className?: string
}

const toneClasses: Record<BadgeTone, string> = {
  neutral: 'bg-subtle border-default text-muted',
  accent: 'bg-accent-subtle border-accent/20 text-accent',
  success: 'bg-success-bg border-success/30 text-success',
  warning: 'bg-warning-bg border-warning/30 text-warning',
  danger: 'bg-danger-bg border-danger/25 text-danger',
}

const dotClasses: Record<BadgeTone, string> = {
  neutral: 'bg-muted',
  accent: 'bg-accent',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
}

const sizeClasses: Record<BadgeSize, string> = {
  sm: 'px-2 py-0.5 text-[11px]',
  md: 'px-2.5 py-1 text-xs',
}

export default function Badge({
  children,
  tone = 'neutral',
  size = 'sm',
  dot = false,
  className = '',
}: BadgeProps) {
  return (
    <span
      className={[
        'inline-flex items-center gap-1.5 rounded-full border font-semibold uppercase tracking-[0.04em]',
        toneClasses[tone],
        sizeClasses[size],
        className,
      ].filter(Boolean).join(' ')}
    >
      {dot && <span className={`size-1.5 rounded-full ${dotClasses[tone]}`} aria-hidden="true" />}
      {children}
    </span>
  )
}
