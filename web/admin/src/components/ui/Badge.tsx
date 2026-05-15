import type { ReactNode } from 'react'

// Non-status tag (trigger source, TTL, hint). Status surfaces use StatusToken
// instead, per DESIGN.md §2.5.
type BadgeTone = 'neutral' | 'accent'

interface BadgeProps {
  tone?: BadgeTone
  children: ReactNode
}

const TONE: Record<BadgeTone, string> = {
  neutral: 'border-default text-secondary bg-surface',
  accent:  'border-default text-accent bg-accent-subtle',
}

export default function Badge({ tone = 'neutral', children }: BadgeProps) {
  return (
    <span
      className={`inline-flex items-center h-5 px-1.5 rounded-sm border font-mono text-[11px] ${TONE[tone]}`}
    >
      {children}
    </span>
  )
}
