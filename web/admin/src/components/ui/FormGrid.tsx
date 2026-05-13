import type { ReactNode } from 'react'

interface FormGridProps {
  columns?: 1 | 2
  children: ReactNode
}

// Grid wrapper for grouped fields. Stays single-column at small widths, splits
// at md+ when columns=2.
export default function FormGrid({ columns = 2, children }: FormGridProps) {
  return (
    <div
      className={`grid grid-cols-1 gap-4 ${columns === 2 ? 'md:grid-cols-2' : ''}`}
    >
      {children}
    </div>
  )
}
