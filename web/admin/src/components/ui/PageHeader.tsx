import type { ReactNode } from 'react'

interface PageHeaderProps {
  title: ReactNode
  actions?: ReactNode
}

export default function PageHeader({ title, actions }: PageHeaderProps) {
  return (
    <div className="flex justify-between items-center gap-4 mb-8">
      <h2 className="text-[28px] font-semibold text-primary -tracking-[0.01em] leading-tight m-0">
        {title}
      </h2>
      {actions}
    </div>
  )
}
