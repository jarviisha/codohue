import type { ReactNode } from 'react'
import PageHeader from './PageHeader'

interface PageShellProps {
  children: ReactNode
  title?: ReactNode
  actions?: ReactNode
  constrained?: boolean
  className?: string
  bodyClassName?: string
}

export default function PageShell({
  children,
  title,
  actions,
  constrained = false,
  className = '',
  bodyClassName = '',
}: PageShellProps) {
  const bodyOffset = title ? 'mt-4 md:mt-0 md:pt-20' : ''

  return (
    <div className={`${constrained ? 'max-w-140' : ''} ${className}`}>
      {title && <PageHeader title={title} actions={actions} />}
      <div className={`flex flex-col gap-4 ${bodyOffset} ${bodyClassName}`}>
        {children}
      </div>
    </div>
  )
}
