import { useNavigate } from 'react-router-dom'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import ErrorBanner from '../components/ErrorBanner'
import { SummaryBar, NamespaceCard } from './namespaces/components'
import { Button, EmptyState, LoadingState, Notice, PageHeader, PageShell } from '../components/ui'
import { useActiveNamespace } from '../context/useActiveNamespace'

export default function NamespacesPage() {
  const { data, error, isLoading } = useNamespacesOverview()
  const { namespace: activeNs, setNamespace } = useActiveNamespace()
  const navigate = useNavigate()

  function handleSelect(ns: string) {
    setNamespace(ns)
    navigate('/health')
  }

  return (
    <PageShell>
      <PageHeader
        title="Namespaces"
        actions={(
          <Button variant="primary" onClick={() => navigate('/namespaces/new')}>
            + Create Namespace
          </Button>
        )}
      />

      {!activeNs && (
        <Notice tone="accent" role="status">
          Select a namespace below to start working.
        </Notice>
      )}

      {error && <ErrorBanner message="Failed to load namespaces." />}
      {isLoading && <LoadingState />}

      {data && data.namespaces.length === 0 && (
        <EmptyState>
          No namespaces yet — create one to get started.
        </EmptyState>
      )}

      {data && data.namespaces.length > 0 && (
        <>
          <SummaryBar namespaces={data.namespaces} />
          <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
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
    </PageShell>
  )
}
