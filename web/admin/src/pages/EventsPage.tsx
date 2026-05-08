import { useState } from 'react'
import { useNamespaceList } from '../hooks/useNamespaces'
import { useEvents } from '../hooks/useEvents'
import { useInjectEvent } from '../hooks/useInjectEvent'
import ErrorBanner from '../components/ErrorBanner'
import { EmptyState, LoadingState, PageHeader, PageShell } from '../components/ui'
import { useActiveNamespace } from '../context/useActiveNamespace'
import EventsTable from './events/EventsTable'
import InjectEventPanel, { type InjectEventPayload } from './events/InjectEventPanel'
import SubjectFilter from './events/SubjectFilter'
import SummaryStrip from './events/SummaryStrip'

const DEFAULT_ACTIONS = ['VIEW', 'LIKE', 'COMMENT', 'SHARE', 'SKIP']
const PAGE_SIZE = 20

export default function EventsPage() {
  const { namespace } = useActiveNamespace()
  const { data: nsData } = useNamespaceList()
  const [offset, setOffset] = useState(0)
  const [subjectFilter, setSubjectFilter] = useState('')
  const [appliedSubject, setAppliedSubject] = useState('')

  const { data, error, isLoading } = useEvents(namespace, PAGE_SIZE, offset, appliedSubject)
  const inject = useInjectEvent(namespace)

  const total = data?.total ?? 0
  const pageEnd = Math.min(offset + PAGE_SIZE, total)

  const nsConfig = nsData?.items.find(n => n.namespace === namespace)
  const actions = nsConfig?.action_weights && Object.keys(nsConfig.action_weights).length > 0
    ? Object.keys(nsConfig.action_weights)
    : DEFAULT_ACTIONS

  function handleApplyFilter() {
    setAppliedSubject(subjectFilter)
    setOffset(0)
  }

  function handleClearFilter() {
    setSubjectFilter('')
    setAppliedSubject('')
    setOffset(0)
  }

  async function handleInject(payload: InjectEventPayload) {
    await inject.mutateAsync(payload)
  }

  return (
    <PageShell>
      <PageHeader title="Events" />

      {namespace && (
        <>
          <InjectEventPanel
            actions={actions}
            errorMessage={inject.error?.message}
            isPending={inject.isPending}
            onInject={handleInject}
          />

          <SubjectFilter
            value={subjectFilter}
            applied={appliedSubject !== ''}
            onChange={setSubjectFilter}
            onApply={handleApplyFilter}
            onClear={handleClearFilter}
          />

          {error && <ErrorBanner message="Failed to load events." />}

          {isLoading && !data && <LoadingState />}

          {data && total > 0 && (
            <SummaryStrip events={data.items} total={total} subjectFilter={appliedSubject} />
          )}

          {data && data.items.length === 0 && (
            <EmptyState>
              No events found. Use the inject form above or send events via the main API.
            </EmptyState>
          )}

          {data && data.items.length > 0 && (
            <EventsTable
              events={data.items}
              offset={offset}
              pageSize={PAGE_SIZE}
              pageEnd={pageEnd}
              total={total}
              subjectFilter={appliedSubject}
              onPreviousPage={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
              onNextPage={() => setOffset(offset + PAGE_SIZE)}
            />
          )}
        </>
      )}
    </PageShell>
  )
}
