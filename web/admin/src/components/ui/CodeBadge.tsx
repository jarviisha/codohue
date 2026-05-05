import type { ReactNode } from 'react'

interface CodeBadgeProps {
  children: ReactNode
  className?: string
}

export default function CodeBadge({ children, className = '' }: CodeBadgeProps) {
  return (
    <code className={`text-[12px] bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm font-medium ${className}`}>
      {children}
    </code>
  )
}
