import type { ReactNode } from 'react'

interface PanelProps {
  title?: ReactNode
  actions?: ReactNode
  footer?: ReactNode
  children: ReactNode
  className?: string
  bodyClassName?: string
}

export default function Panel({
  title,
  actions,
  footer,
  children,
  className = '',
  bodyClassName = '',
}: PanelProps) {
  return (
    <div className={`bg-surface border border-default rounded-lg ${className}`}>
      {(title || actions) && (
        <div className="flex items-center justify-between gap-4 border-b border-default px-5 py-4">
          {title && (
            <h3 className="m-0 text-sm font-semibold text-primary">
              {title}
            </h3>
          )}
          {actions}
        </div>
      )}
      <div className={`p-5 ${bodyClassName}`}>{children}</div>
      {footer && <div className="border-t border-default px-5 py-4">{footer}</div>}
    </div>
  )
}
