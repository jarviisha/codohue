import type { ReactNode } from 'react'

interface PageShellProps {
  children: ReactNode
  constrained?: boolean
  className?: string
}

export default function PageShell({
  children,
  constrained = false,
  className = '',
}: PageShellProps) {
  return (
    <div className={`${constrained ? 'max-w-140' : ''} flex flex-col gap-6 ${className}`}>
      {children}
    </div>
  )
}
