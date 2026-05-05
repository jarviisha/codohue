import { useNavigate } from 'react-router-dom'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import ErrorBanner from '../components/ErrorBanner'
import { SummaryBar, NamespaceCard } from './namespaces/components'
import { Button, EmptyState, PageHeader } from '../components/ui'
import { useActiveNamespace } from '../context/NamespaceContext'

export default function NamespacesPage() {
  const { data, error, isLoading } = useNamespacesOverview()
  const { namespace: activeNs, setNamespace } = useActiveNamespace()
  const navigate = useNavigate()

  function handleSelect(ns: string) {
    setNamespace(ns)
    navigate('/health')
  }

  return (
    <div>
      <PageHeader
        title="Namespaces"
        actions={(
          <Button variant="primary" onClick={() => navigate('/namespaces/new')}>
            + Create Namespace
          </Button>
        )}
      />

      {!activeNs && (
        <div className="flex items-center gap-3 px-4 py-3 mb-6 rounded-xl bg-accent-subtle border border-accent/20 text-sm text-accent font-medium">
          Select a namespace below to start working.
        </div>
      )}

      {error && <ErrorBanner message="Failed to load namespaces." />}
      {isLoading && <p className="text-sm text-muted">Loading…</p>}

      {data && data.namespaces.length === 0 && (
        <EmptyState>
          No namespaces yet — create one to get started.
        </EmptyState>
      )}

      {data && data.namespaces.length > 0 && (
        <>
          <SummaryBar namespaces={data.namespaces} />
          <div className="grid grid-cols-2 gap-4">
            {data.namespaces.map(h => (
              <NamespaceCard
                key={h.config.namespace}
                health={h}
                isActive={h.config.namespace === activeNs}
                onSelect={() => handleSelect(h.config.namespace)}
                onEdit={() => navigate(`/namespaces/${h.config.namespace}`)}
              />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
