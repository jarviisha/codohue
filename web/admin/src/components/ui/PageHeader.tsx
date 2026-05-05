import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: ReactNode
  actions?: ReactNode
}

export default function PageHeader({ title, actions }: PageHeaderProps) {
  return (
    <div className="flex justify-between items-center gap-4 mb-8 border-b pb-3 border-default">
      <h2 className="text-2xl font-bold text-primary tracking-[-0.01em] leading-tight m-0">
        {title}
      </h2>
      {actions}
    </div>
  )
}
