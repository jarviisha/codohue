import type { ReactNode } from 'react'
import Breadcrumbs from '../Breadcrumbs'
import { appLayoutClasses } from '../appLayoutClasses'

interface PageHeaderProps {
  title: ReactNode
  actions?: ReactNode
}

export default function PageHeader({ title, actions }: PageHeaderProps) {
  return (
    <div className={appLayoutClasses.pageHeader}>
      <Breadcrumbs className="min-h-4" />
      <div className="flex items-center justify-between gap-3">
        <h2 className="m-0 min-w-0 text-xl font-semibold leading-tight text-primary">
          {title}
        </h2>
        {actions && <div className="shrink-0">{actions}</div>}
      </div>
    </div>
  )
}
