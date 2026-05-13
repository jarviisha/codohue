import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: ReactNode
  actions?: ReactNode
}

export default function PageHeader({ title, actions }: PageHeaderProps) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-default pb-2">
      <h2 className="m-0 text-xl font-semibold leading-tight text-primary">
        {title}
      </h2>
      {actions}
    </div>
  )
}
