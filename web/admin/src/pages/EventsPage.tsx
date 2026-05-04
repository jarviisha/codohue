import { useState } from 'react'
import { useNamespaceList } from '../hooks/useNamespaces'
import { useEvents } from '../hooks/useEvents'
import { useInjectEvent } from '../hooks/useInjectEvent'
import ErrorBanner from '../components/ErrorBanner'

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
  const { data: nsData } = useNamespaceList()
  const [namespace, setNamespace] = useState('')
  const [offset, setOffset] = useState(0)
  const [subjectFilter, setSubjectFilter] = useState('')
  const [appliedSubject, setAppliedSubject] = useState('')

  // Inject form state
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
      {/* Page header */}
      <div className="flex items-center gap-4 mb-8">
        <h2 className="text-[28px] font-semibold text-primary -tracking-[0.01em] leading-tight m-0">
          Events
        </h2>
        <select
          value={namespace}
          onChange={e => { setNamespace(e.target.value); setOffset(0); setAppliedSubject(''); setSubjectFilter('') }}
          className="bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary text-sm px-3 py-2 rounded-md focus:outline-none transition-shadow duration-100"
        >
          <option value="">Select namespace</option>
          {nsData?.namespaces.map(ns => (
            <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
          ))}
        </select>
      </div>

      {!namespace && (
        <p className="text-sm text-muted">Select a namespace to view and inject events.</p>
      )}

      {namespace && (
        <>
          {/* Inject form */}
          <div className="bg-surface border border-default rounded-lg p-5 mb-6">
            <h3 className="text-sm font-semibold text-primary mb-4 m-0">Inject Test Event</h3>
            {inject.error && <ErrorBanner message={inject.error.message} />}
            {injectSuccess && (
              <div className="flex items-center gap-2 px-4 py-3 mb-4 rounded-lg bg-success-bg border border-success/30 text-success text-sm font-medium">
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
                  className="bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary placeholder:text-muted text-sm px-3 py-2.5 rounded-md focus:outline-none transition-shadow duration-100 w-48"
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
                  className="bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary placeholder:text-muted text-sm px-3 py-2.5 rounded-md focus:outline-none transition-shadow duration-100 w-48"
                  required
                />
              </div>
              <div className="flex flex-col gap-1">
                <label className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Action</label>
                <select
                  value={injectAction}
                  onChange={e => setInjectAction(e.target.value)}
                  className="bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary text-sm px-3 py-2.5 rounded-md focus:outline-none transition-shadow duration-100"
                >
                  {actions.map(a => <option key={a} value={a}>{a}</option>)}
                </select>
              </div>
              <button
                type="submit"
                disabled={inject.isPending || !injectSubject.trim() || !injectObject.trim()}
                className="bg-accent hover:bg-accent-hover active:bg-accent-active text-accent-text font-medium text-sm px-5 py-2.5 rounded-md disabled:opacity-50 disabled:cursor-not-allowed transition-colors duration-150"
              >
                {inject.isPending ? 'Injecting…' : 'Inject Event'}
              </button>
            </form>
          </div>

          {/* Filters */}
          <div className="flex gap-3 items-end mb-4">
            <div className="flex flex-col gap-1">
              <label className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Filter by Subject ID</label>
              <input
                type="text"
                value={subjectFilter}
                onChange={e => setSubjectFilter(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleApplyFilter()}
                placeholder="user-1"
                className="bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary placeholder:text-muted text-sm px-3 py-2 rounded-md focus:outline-none transition-shadow duration-100 w-48"
              />
            </div>
            <button
              onClick={handleApplyFilter}
              className="bg-accent hover:bg-accent-hover active:bg-accent-active text-accent-text font-medium text-sm px-4 py-2 rounded-md transition-colors duration-150"
            >
              Apply
            </button>
            {appliedSubject && (
              <button
                onClick={handleClearFilter}
                className="text-sm font-medium text-secondary hover:text-primary transition-colors duration-150 px-2 py-2"
              >
                Clear
              </button>
            )}
          </div>

          {error && <ErrorBanner message="Failed to load events." />}

          {/* Summary row */}
          {data && (
            <div className="flex items-center justify-between mb-3">
              <p className="text-xs text-muted m-0">
                {total === 0 ? 'No events' : `Showing ${pageStart}–${pageEnd} of ${total.toLocaleString()} total`}
                {appliedSubject && <span className="ml-1">for subject <code className="font-mono text-[12px] bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm">{appliedSubject}</code></span>}
                {isFetching && <span className="ml-2 text-muted">Refreshing…</span>}
              </p>
            </div>
          )}

          {isLoading && !data && <p className="text-sm text-muted">Loading…</p>}

          {data && data.events.length === 0 && (
            <div className="p-10 text-center text-sm text-muted border border-dashed border-default rounded-lg">
              No events found. Use the inject form above or send events via the main API.
            </div>
          )}

          {data && data.events.length > 0 && (
            <div className="bg-surface border border-default rounded-lg overflow-hidden">
              <table className="w-full border-collapse">
                <thead>
                  <tr className="bg-subtle border-b-2 border-default">
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Time</th>
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Subject ID</th>
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Object ID</th>
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Action</th>
                    <th className="px-4 py-2.5 text-right text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Weight</th>
                  </tr>
                </thead>
                <tbody>
                  {data.events.map(ev => (
                    <tr key={ev.id} className="border-b border-default hover:bg-surface-raised">
                      <td className="px-4 py-2.5 text-xs text-muted tabular-nums whitespace-nowrap">{formatTime(ev.occurred_at)}</td>
                      <td className="px-4 py-2.5 text-sm">
                        <code className="font-mono text-[12px] bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm">{ev.subject_id}</code>
                      </td>
                      <td className="px-4 py-2.5 text-sm">
                        <code className="font-mono text-[12px] bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm">{ev.object_id}</code>
                      </td>
                      <td className="px-4 py-2.5 text-sm text-primary font-medium">{ev.action}</td>
                      <td className="px-4 py-2.5 text-sm text-primary text-right tabular-nums">{ev.weight.toFixed(2)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Pagination */}
          {data && total > PAGE_SIZE && (
            <div className="flex items-center justify-between mt-4">
              <button
                onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                disabled={offset === 0}
                className="text-sm font-medium px-4 py-2 rounded-md border border-default hover:border-strong hover:bg-surface-raised text-secondary disabled:opacity-40 disabled:cursor-not-allowed transition-colors duration-150"
              >
                ← Prev
              </button>
              <button
                onClick={() => setOffset(offset + PAGE_SIZE)}
                disabled={pageEnd >= total}
                className="text-sm font-medium px-4 py-2 rounded-md border border-default hover:border-strong hover:bg-surface-raised text-secondary disabled:opacity-40 disabled:cursor-not-allowed transition-colors duration-150"
              >
                Next →
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
