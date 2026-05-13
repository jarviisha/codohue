import type { ReactNode } from 'react'

interface CodeBadgeProps {
  children: ReactNode
  className?: string
}

export default function CodeBadge({ children, className = '' }: CodeBadgeProps) {
  return (
    <code className={`rounded border border-default bg-subtle px-1.5 py-0.5 text-[12px] font-medium text-primary ${className}`}>
      {children}
    </code>
  )
}
