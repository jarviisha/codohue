import { useState } from 'react'
import { useNamespaceList } from '../hooks/useNamespaces'
import { useEvents } from '../hooks/useEvents'
import { useInjectEvent } from '../hooks/useInjectEvent'
import ErrorBanner from '../components/ErrorBanner'
import { Button, CodeBadge, EmptyState, PageHeader, Panel, Table, Thead, Th, Tbody, Tr, Td, inputClass } from '../components/ui'
import type { EventSummary } from '../types'
import { useActiveNamespace } from '../context/NamespaceContext'

const DEFAULT_ACTIONS = ['VIEW', 'LIKE', 'COMMENT', 'SHARE', 'SKIP']
const PAGE_SIZE = 50

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

export default function EventsPage() {
  const { namespace } = useActiveNamespace()
  const { data: nsData } = useNamespaceList()
  const [offset, setOffset] = useState(0)
  const [subjectFilter, setSubjectFilter] = useState('')
  const [appliedSubject, setAppliedSubject] = useState('')
  const [injectSubject, setInjectSubject] = useState('')
  const [injectObject, setInjectObject] = useState('')
  const [injectAction, setInjectAction] = useState('VIEW')
  const [injectSuccess, setInjectSuccess] = useState(false)

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

  async function handleInjectSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!injectSubject.trim() || !injectObject.trim()) return
    try {
      await inject.mutateAsync({ subject_id: injectSubject.trim(), object_id: injectObject.trim(), action: injectAction })
      setInjectSuccess(true)
      setInjectSubject('')
      setInjectObject('')
      setTimeout(() => setInjectSuccess(false), 3000)
    } catch {
      // error shown via inject.error
    }
  }

  return (
    <div>
      <PageHeader title="Events" />

      {namespace && (
        <>
          <Panel title="Inject Test Event" className="mb-6">
            {inject.error && <ErrorBanner message={inject.error.message} />}
            {injectSuccess && (
              <div className="flex items-center gap-2 px-4 py-3 mb-4 rounded-xl bg-success-bg border border-success/30 text-success text-sm font-medium">
                Event injected successfully.
              </div>
            )}
            <form onSubmit={handleInjectSubmit} className="flex flex-wrap gap-3 items-end">
              <div className="flex flex-col gap-1">
                <label className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Subject ID</label>
                <input
                  type="text"
                  value={injectSubject}
                  onChange={e => setInjectSubject(e.target.value)}
                  placeholder="user-1"
                  className={`${inputClass} w-48 py-2.5`}
                  required
                />
              </div>
              <div className="flex flex-col gap-1">
                <label className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Object ID</label>
                <input
                  type="text"
                  value={injectObject}
                  onChange={e => setInjectObject(e.target.value)}
                  placeholder="item-42"
                  className={`${inputClass} w-48 py-2.5`}
                  required
                />
              </div>
              <div className="flex flex-col gap-1">
                <label className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Action</label>
                <select
                  value={injectAction}
                  onChange={e => setInjectAction(e.target.value)}
                  className={`${inputClass} py-2.5`}
                >
                  {actions.map(a => <option key={a} value={a}>{a}</option>)}
                </select>
              </div>
              <Button
                type="submit"
                variant="primary"
                disabled={inject.isPending || !injectSubject.trim() || !injectObject.trim()}
              >
                {inject.isPending ? 'Injecting…' : 'Inject Event'}
              </Button>
            </form>
          </Panel>

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
                {total === 0 ? 'No events' : `Showing ${pageStart}–${pageEnd} of ${total.toLocaleString()} total`}
                {appliedSubject && <span className="ml-1">for subject <CodeBadge>{appliedSubject}</CodeBadge></span>}
                {isFetching && <span className="ml-2 text-muted">Refreshing…</span>}
              </p>
            </div>
          )}

          {isLoading && !data && <p className="text-sm text-muted">Loading…</p>}

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
    </div>
  )
}

interface SubjectFilterProps {
  value: string
  applied: boolean
  onChange: (value: string) => void
  onApply: () => void
  onClear: () => void
}

function SubjectFilter({ value, applied, onChange, onApply, onClear }: SubjectFilterProps) {
  return (
    <div className="flex gap-3 items-end mb-4">
      <div className="flex flex-col gap-1">
        <label className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Filter by Subject ID</label>
        <input
          type="text"
          value={value}
          onChange={e => onChange(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && onApply()}
          placeholder="user-1"
          className={`${inputClass} w-48`}
        />
      </div>
      <Button onClick={onApply} variant="primary" size="sm">
        Apply
      </Button>
      {applied && (
        <Button onClick={onClear} variant="ghost" size="sm">
          Clear
        </Button>
      )}
    </div>
  )
}

function EventsTable({ events }: { events: EventSummary[] }) {
  return (
    <Panel>
      <Table>
        <Thead>
          <Th>Time</Th>
          <Th>Subject ID</Th>
          <Th>Object ID</Th>
          <Th>Action</Th>
          <Th align="right">Weight</Th>
        </Thead>
        <Tbody>
          {events.map(ev => (
            <Tr key={ev.id} hoverable>
              <Td muted mono className="whitespace-nowrap">{formatTime(ev.occurred_at)}</Td>
              <Td><CodeBadge>{ev.subject_id}</CodeBadge></Td>
              <Td><CodeBadge>{ev.object_id}</CodeBadge></Td>
              <Td className="font-medium">{ev.action}</Td>
              <Td align="right" mono>{ev.weight.toFixed(2)}</Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </Panel>
  )
}

function EventsPagination({
  offset,
  pageEnd,
  total,
  onPrevious,
  onNext,
}: {
  offset: number
  pageEnd: number
  total: number
  onPrevious: () => void
  onNext: () => void
}) {
  return (
    <div className="flex items-center justify-between mt-4">
      <Button onClick={onPrevious} disabled={offset === 0}>
        ← Prev
      </Button>
      <Button onClick={onNext} disabled={pageEnd >= total}>
        Next →
      </Button>
    </div>
  )
}
