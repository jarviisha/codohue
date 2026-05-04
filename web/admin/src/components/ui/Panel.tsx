import type { ReactNode } from 'react'

interface PanelProps {
  title?: ReactNode
  children: ReactNode
  className?: string
}

export default function Panel({ title, children, className = '' }: PanelProps) {
  return (
    <div className={`bg-surface border border-default rounded-lg p-5 ${className}`}>
      {title && (
        <h3 className="text-sm font-semibold text-primary mb-4 m-0">
          {title}
        </h3>
      )}
      {children}
    </div>
  )
}
