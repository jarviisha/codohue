import { Link, useLocation } from 'react-router-dom'
import { adminRoutes } from '../routes'

type Crumb = { label: string; to: string }

const routeLabelByPath = new Map(adminRoutes.map(route => [`/${route.path}`, route.label]))

function labelFromSegment(segment: string) {
  const cleaned = decodeURIComponent(segment).replace(/-/g, ' ')
  return cleaned
    .split(' ')
    .filter(Boolean)
    .map(word => word.slice(0, 1).toUpperCase() + word.slice(1))
    .join(' ')
}

function buildBreadcrumbs(pathname: string): Crumb[] {
  const normalized = pathname === '/' ? '/health' : pathname
  const segments = normalized.split('/').filter(Boolean)

  return segments.map((segment, index) => {
    const to = `/${segments.slice(0, index + 1).join('/')}`
    const label = routeLabelByPath.get(to) ?? labelFromSegment(segment)
    return { label, to }
  })
}

export default function Breadcrumbs() {
  const location = useLocation()
  const crumbs = buildBreadcrumbs(location.pathname)

  if (crumbs.length <= 1) return null

  return (
    <nav aria-label="Breadcrumb" className="mb-4">
      <ol className="m-0 flex list-none items-center gap-2 p-0 text-xs text-muted">
        {crumbs.map((crumb, index) => {
          const isLast = index === crumbs.length - 1
          return (
            <li key={crumb.to} className="flex items-center gap-2">
              {isLast ? (
                <span className="text-secondary font-medium">{crumb.label}</span>
              ) : (
                <Link
                  to={crumb.to}
                  className="rounded text-muted no-underline transition-colors duration-150 hover:text-primary focus-visible:outline-none focus-visible:shadow-focus"
                >
                  {crumb.label}
                </Link>
              )}
              {!isLast && <span className="text-muted">/</span>}
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
