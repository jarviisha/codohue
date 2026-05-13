import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import ErrorBanner from '../components/ErrorBanner'
import { SummaryBar } from './namespaces/components'
import {
  Button,
  CodeBadge,
  EmptyState,
  FormControl,
  Input,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  Panel,
  Select,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Toolbar,
  Tr,
} from '../components/ui'
import { useActiveNamespace } from '../context/useActiveNamespace'
import type { NamespaceHealth, NamespaceStatus } from '../types'
import { formatCount } from '../utils/format'
import LastRunSummary from './namespaces/LastRunSummary'
import RunNowButton from './namespaces/RunNowButton'
import StatusBadge from './namespaces/StatusBadge'

const setupSteps = [
  'Create namespace config',
  'Use the generated namespace API key',
  'Send events to start recommendations',
]

const STATUS_OPTIONS: Array<{ label: string; value: NamespaceStatus | 'all' }> = [
  { label: 'All statuses', value: 'all' },
  { label: 'Active', value: 'active' },
  { label: 'Idle', value: 'idle' },
  { label: 'Degraded', value: 'degraded' },
  { label: 'Cold', value: 'cold' },
]

export default function NamespacesPage() {
  const { data, error, isLoading } = useNamespacesOverview()
  const { namespace: activeNs, setNamespace } = useActiveNamespace()
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<NamespaceStatus | 'all'>('all')
  const hasNamespaces = (data?.items.length ?? 0) > 0
  const filteredNamespaces = useMemo(() => {
    const q = query.trim().toLowerCase()
    return (data?.items ?? []).filter(item => {
      const matchesQuery = !q || item.config.namespace.toLowerCase().includes(q)
      const matchesStatus = statusFilter === 'all' || item.status === statusFilter
      return matchesQuery && matchesStatus
    })
  }, [data?.items, query, statusFilter])

  function handleSelect(ns: string) {
    setNamespace(ns)
    navigate(`/namespaces/${encodeURIComponent(ns)}/overview`)
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

      {error && <ErrorBanner message="Failed to load namespaces." />}
      {isLoading && <LoadingState />}

      {data && data.items.length === 0 && (
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
          </div>

          <div className="grid grid-cols-1 gap-3 border-t border-default pt-4 md:grid-cols-3">
            {setupSteps.map((step, index) => (
              <div key={step} className="flex items-center gap-3 rounded border border-default bg-subtle px-3 py-2">
                <span className="flex size-6 shrink-0 items-center justify-center rounded bg-surface text-xs font-semibold text-primary">
                  {index + 1}
                </span>
                <span className="text-sm font-medium text-secondary">{step}</span>
              </div>
            ))}
          </div>

          <Notice tone="accent" role="status">
            Demo data is managed from namespace settings. Create a namespace first, then open Settings.
          </Notice>
        </Panel>
      )}

      {data && data.items.length > 0 && (
        <>
          <SummaryBar namespaces={data.items} />
          <Panel title="Namespace Inventory">
            <Toolbar className="mb-4">
              <FormControl label="Search" htmlFor="namespace-search">
                <Input
                  id="namespace-search"
                  inputSize="sm"
                  value={query}
                  onChange={e => setQuery(e.target.value)}
                  placeholder="Filter by namespace"
                  className="w-full sm:w-64"
                />
              </FormControl>
              <FormControl label="Status" htmlFor="namespace-status">
                <Select
                  id="namespace-status"
                  controlSize="sm"
                  value={statusFilter}
                  onChange={e => setStatusFilter(e.target.value as NamespaceStatus | 'all')}
                  options={STATUS_OPTIONS}
                  className="w-40"
                />
              </FormControl>
              {(query || statusFilter !== 'all') && (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setQuery('')
                    setStatusFilter('all')
                  }}
                >
                  Clear
                </Button>
              )}
            </Toolbar>

            {filteredNamespaces.length === 0 ? (
              <EmptyState className="p-6">
                No namespaces match the current filters.
              </EmptyState>
            ) : (
              <NamespaceTable
                namespaces={filteredNamespaces}
                activeNs={activeNs}
                onSelect={handleSelect}
                onEdit={ns => navigate(`/namespaces/${encodeURIComponent(ns)}/settings`)}
              />
            )}
          </Panel>
        </>
      )}
    </PageShell>
  )
}

function NamespaceTable({
  namespaces,
  activeNs,
  onSelect,
  onEdit,
}: {
  namespaces: NamespaceHealth[]
  activeNs: string
  onSelect: (ns: string) => void
  onEdit: (ns: string) => void
}) {
  return (
    <div className="overflow-x-auto">
      <Table>
        <Thead>
          <Th>Namespace</Th>
          <Th>Status</Th>
          <Th align="right">Events 24h</Th>
          <Th>Strategy</Th>
          <Th align="right">Max Results</Th>
          <Th>Last Run</Th>
          <Th align="right">Actions</Th>
        </Thead>
        <Tbody>
          {namespaces.map(health => {
            const ns = health.config.namespace
            const isActive = ns === activeNs
            return (
              <Tr key={ns} hoverable>
                <Td>
                  <div className="flex items-center gap-2">
                    <CodeBadge className="text-primary">{ns}</CodeBadge>
                    {isActive && (
                      <span className="rounded-full bg-accent-subtle px-2 py-0.5 text-[11px] font-semibold uppercase text-accent">
                        Active
                      </span>
                    )}
                  </div>
                </Td>
                <Td>
                  <StatusBadge status={health.status} />
                </Td>
                <Td align="right" mono>
                  {formatCount(health.active_events_24h)}
                </Td>
                <Td muted mono>
                  {health.config.dense_strategy || '—'}
                </Td>
                <Td align="right" mono>
                  {health.config.max_results}
                </Td>
                <Td>
                  <LastRunSummary health={health} />
                </Td>
                <Td align="right">
                  <div className="flex justify-end gap-1">
                    <Button
                      type="button"
                      size="sm"
                      variant={isActive ? 'secondary' : 'primary'}
                      disabled={isActive}
                      onClick={() => onSelect(ns)}
                    >
                      {isActive ? 'Selected' : 'Open'}
                    </Button>
                    <Button type="button" size="sm" onClick={() => onEdit(ns)}>
                      Settings
                    </Button>
                    <RunNowButton ns={ns} />
                  </div>
                </Td>
              </Tr>
            )
          })}
        </Tbody>
      </Table>
    </div>
  )
}
