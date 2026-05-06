import { useNavigate } from 'react-router-dom'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import { useClearDemoDataset, useSeedDemoDataset } from '../hooks/useDemoDataset'
import ErrorBanner from '../components/ErrorBanner'
import { SummaryBar, NamespaceCard } from './namespaces/components'
import { Button, CodeBadge, LoadingState, Notice, PageHeader, PageShell, Panel } from '../components/ui'
import { useActiveNamespace } from '../context/useActiveNamespace'

const setupSteps = [
  'Create namespace config',
  'Use the generated namespace API key',
  'Send events to start recommendations',
]

export default function NamespacesPage() {
  const { data, error, isLoading } = useNamespacesOverview()
  const { namespace: activeNs, setNamespace } = useActiveNamespace()
  const seedDemo = useSeedDemoDataset()
  const clearDemo = useClearDemoDataset()
  const navigate = useNavigate()
  const demoExists = data?.namespaces.some(h => h.config.namespace === 'demo') ?? false
  const hasNamespaces = (data?.namespaces.length ?? 0) > 0

  function handleSelect(ns: string) {
    setNamespace(ns)
    navigate('/health')
  }

  function handleSeedDemo() {
    seedDemo.mutate(undefined, {
      onSuccess: () => {
        setNamespace('demo')
      },
    })
  }

  function handleClearDemo() {
    if (!window.confirm('Clear the demo namespace and all demo data?')) return
    clearDemo.mutate(undefined, {
      onSuccess: () => {
        if (activeNs === 'demo') setNamespace('')
      },
    })
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

      {!activeNs && hasNamespaces && (
        <Notice tone="accent" role="status">
          Select a namespace below to start working.
        </Notice>
      )}

      {hasNamespaces && (
        <section className="flex flex-col gap-3 border-y border-default py-4 md:flex-row md:items-center md:justify-between">
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-primary">Demo Dataset</h2>
            <p className="mt-1 text-sm text-secondary">
              Seed namespace <CodeBadge>demo</CodeBadge> with sample users, items, and interactions for local testing.
            </p>
            {seedDemo.data?.api_key && (
              <p className="mt-2 break-all font-mono text-xs text-secondary">
                Namespace key: {seedDemo.data.api_key}
              </p>
            )}
            {seedDemo.isError && (
              <p className="mt-2 text-sm font-medium text-danger">Failed to seed demo dataset.</p>
            )}
            {clearDemo.isError && (
              <p className="mt-2 text-sm font-medium text-danger">Failed to clear demo dataset.</p>
            )}
            {seedDemo.data && !seedDemo.isError && (
              <p className="mt-2 text-sm font-medium text-success">
                Seeded {seedDemo.data.events_created ?? 0} demo events.
              </p>
            )}
            {clearDemo.data && !clearDemo.isError && (
              <p className="mt-2 text-sm font-medium text-success">
                Cleared {clearDemo.data.events_deleted ?? 0} demo events.
              </p>
            )}
          </div>
          <div className="flex shrink-0 gap-2">
            <Button
              type="button"
              variant="secondary"
              disabled={seedDemo.isPending || clearDemo.isPending}
              onClick={handleSeedDemo}
            >
              {demoExists ? 'Reset Demo' : 'Add Demo'}
            </Button>
            <Button
              type="button"
              variant="danger"
              disabled={!demoExists || seedDemo.isPending || clearDemo.isPending}
              onClick={handleClearDemo}
            >
              Clear Demo
            </Button>
          </div>
        </section>
      )}

      {error && <ErrorBanner message="Failed to load namespaces." />}
      {isLoading && <LoadingState />}

      {data && data.namespaces.length === 0 && (
        <Panel bodyClassName="flex flex-col gap-5">
          <div className="max-w-2xl">
            <h2 className="m-0 text-lg font-semibold text-primary">No namespaces yet</h2>
            <p className="mt-2 text-sm text-secondary">
              Create a namespace to configure recommendation behavior, then use its API key to ingest events.
            </p>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button variant="primary" onClick={() => navigate('/namespaces/new')}>
              Create Namespace
            </Button>
            <Button
              type="button"
              variant="secondary"
              disabled={seedDemo.isPending}
              onClick={handleSeedDemo}
            >
              {seedDemo.isPending ? 'Adding Demo...' : 'Add Demo Dataset'}
            </Button>
            {seedDemo.data && (
              <Button type="button" variant="ghost" onClick={() => navigate('/health')}>
                Open Demo Namespace
              </Button>
            )}
          </div>

          <div className="grid grid-cols-1 gap-3 border-t border-default pt-4 md:grid-cols-3">
            {setupSteps.map((step, index) => (
              <div key={step} className="flex items-center gap-3 rounded border border-default bg-subtle px-3 py-2">
                <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-surface text-xs font-semibold text-primary">
                  {index + 1}
                </span>
                <span className="text-sm font-medium text-secondary">{step}</span>
              </div>
            ))}
          </div>

          {seedDemo.data?.api_key && (
            <Notice tone="success" role="status">
              <span className="block font-medium text-primary">Demo namespace created.</span>
              <span className="mt-1 block break-all font-mono text-xs text-secondary">
                Namespace key: {seedDemo.data.api_key}
              </span>
            </Notice>
          )}
          {seedDemo.isError && (
            <Notice tone="danger" role="alert">
              Failed to seed demo dataset.
            </Notice>
          )}
        </Panel>
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
