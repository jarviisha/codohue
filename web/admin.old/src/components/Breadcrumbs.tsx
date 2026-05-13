import { Link, useLocation } from 'react-router-dom'

type Crumb = { label: string; to?: string }

const namespaceSectionLabels: Record<string, string> = {
  overview: 'Overview',
  'batch-runs': 'Batch Runs',
  events: 'Events',
  trending: 'Trending',
  debug: 'Recommend Debug',
  settings: 'Edit Namespace',
  'catalog/items': 'Catalog Items',
}

function decodePathPart(value: string) {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function labelFromPath(path: string) {
  const cleaned = decodePathPart(path.split('/').at(-1) ?? path).replace(/-/g, ' ')
  return cleaned
    .split(' ')
    .filter(Boolean)
    .map(word => word.slice(0, 1).toUpperCase() + word.slice(1))
    .join(' ')
}

function buildBreadcrumbs(pathname: string): Crumb[] {
  const normalized = pathname === '/' ? '/health' : pathname

  if (normalized === '/health') return [{ label: 'System Health' }]
  if (normalized === '/namespaces') return [{ label: 'Namespaces' }]
  if (normalized === '/namespaces/new') {
    return [
      { label: 'Namespaces', to: '/namespaces' },
      { label: 'Create Namespace' },
    ]
  }

  const namespaceMatch = normalized.match(/^\/namespaces\/([^/]+)(?:\/(.+))?$/)
  if (namespaceMatch) {
    const [, rawNamespace, rawSection = 'overview'] = namespaceMatch
    const namespacePath = `/namespaces/${rawNamespace}/overview`
    const section = namespaceSectionLabels[rawSection] ?? labelFromPath(rawSection)

    return [
      { label: 'Namespaces', to: '/namespaces' },
      {
        label: decodePathPart(rawNamespace),
        to: rawSection === 'overview' ? undefined : namespacePath,
      },
      { label: section },
    ]
  }

  return normalized
    .split('/')
    .filter(Boolean)
    .map(segment => ({ label: labelFromPath(segment) }))
}

export default function Breadcrumbs({ className = 'mb-4' }: { className?: string }) {
  const location = useLocation()
  const crumbs = buildBreadcrumbs(location.pathname)

  if (crumbs.length <= 1) return null

  return (
    <nav aria-label="Breadcrumb" className={className}>
      <ol className="m-0 flex list-none items-center gap-2 p-0 text-xs text-muted">
        {crumbs.map((crumb, index) => {
          const isLast = index === crumbs.length - 1
          return (
            <li key={`${crumb.label}-${index}`} className="flex items-center gap-2">
              {isLast || !crumb.to ? (
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
