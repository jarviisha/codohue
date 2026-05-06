import { useState } from 'react'
import { useNamespaceList } from '../hooks/useNamespaces'
import { useEvents } from '../hooks/useEvents'
import { useInjectEvent } from '../hooks/useInjectEvent'
import ErrorBanner from '../components/ErrorBanner'
import { CodeBadge, EmptyState, LoadingState, PageHeader, PageShell } from '../components/ui'
import { useActiveNamespace } from '../context/useActiveNamespace'
import EventsPagination from './events/EventsPagination'
import EventsTable from './events/EventsTable'
import InjectEventPanel, { type InjectEventPayload } from './events/InjectEventPanel'
import SubjectFilter from './events/SubjectFilter'
import { formatCount } from '../utils/format'

const DEFAULT_ACTIONS = ['VIEW', 'LIKE', 'COMMENT', 'SHARE', 'SKIP']
const PAGE_SIZE = 50

export default function EventsPage() {
  const { namespace } = useActiveNamespace()
  const { data: nsData } = useNamespaceList()
  const [offset, setOffset] = useState(0)
  const [subjectFilter, setSubjectFilter] = useState('')
  const [appliedSubject, setAppliedSubject] = useState('')

  const { data, error, isLoading, isFetching } = useEvents(namespace, PAGE_SIZE, offset, appliedSubject)
  const inject = useInjectEvent(namespace)

  const total = data?.total ?? 0
  const pageStart = total === 0 ? 0 : offset + 1
  const pageEnd = Math.min(offset + PAGE_SIZE, total)

  const nsConfig = nsData?.namespaces.find(n => n.namespace === namespace)
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

          {data && (
            <div className="flex items-center justify-between mb-3">
              <p className="text-xs text-muted m-0">
                {total === 0 ? 'No events' : `Showing ${pageStart}–${pageEnd} of ${formatCount(total)} total`}
                {appliedSubject && <span className="ml-1">for subject <CodeBadge>{appliedSubject}</CodeBadge></span>}
                {isFetching && <span className="ml-2 text-muted">Refreshing...</span>}
              </p>
            </div>
          )}

          {isLoading && !data && <LoadingState />}

          {data && data.events.length === 0 && (
            <EmptyState>
              No events found. Use the inject form above or send events via the main API.
            </EmptyState>
          )}

          {data && data.events.length > 0 && (
            <EventsTable events={data.events} />
          )}

          {data && total > PAGE_SIZE && (
            <EventsPagination
              offset={offset}
              pageEnd={pageEnd}
              total={total}
              onPrevious={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
              onNext={() => setOffset(offset + PAGE_SIZE)}
            />
          )}
        </>
      )}
    </PageShell>
  )
}
