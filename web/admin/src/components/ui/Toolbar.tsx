import type { ReactNode } from 'react'

interface ToolbarProps {
  children: ReactNode
  className?: string
}

export default function Toolbar({ children, className = '' }: ToolbarProps) {
  return (
    <div className={`flex flex-wrap items-end gap-3 ${className}`}>
      {children}
    </div>
  )
}
