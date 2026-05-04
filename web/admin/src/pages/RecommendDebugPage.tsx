import { useState } from 'react'
import { useRecommendDebug } from '../hooks/useRecommendDebug'
import { useSubjectProfile } from '../hooks/useSubjectProfile'
import { useNamespaceList } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

const LIMITS = [5, 10, 20, 50]

const inputClass = 'bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary placeholder:text-muted text-sm px-3 py-2 rounded-md focus:outline-none transition-shadow duration-100'

export default function RecommendDebugPage() {
  const { data: nsData } = useNamespaceList()
  const debug = useRecommendDebug()
  const profile = useSubjectProfile()

  const [namespace, setNamespace] = useState('')
  const [subjectID, setSubjectID] = useState('')
  const [limit, setLimit] = useState(10)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    debug.mutate({ namespace, subject_id: subjectID, limit, offset: 0 })
    profile.mutate({ namespace, subject_id: subjectID })
  }

  const isPending = debug.isPending || profile.isPending

  return (
    <div>
      <h2 className="text-[28px] font-semibold text-primary -tracking-[0.01em] leading-tight m-0 mb-8">
        Recommendation Debug
      </h2>

      <form
        onSubmit={handleSubmit}
        className="bg-surface border border-default rounded-lg flex gap-4 flex-wrap items-end p-5 mb-6"
      >
        <div>
          <label className="block text-[13px] font-medium text-primary mb-1.5">Namespace</label>
          <select required value={namespace} onChange={e => setNamespace(e.target.value)} className={inputClass}>
            <option value="">Select namespace</option>
            {nsData?.namespaces.map(ns => (
              <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="block text-[13px] font-medium text-primary mb-1.5">Subject ID</label>
          <input
            required
            value={subjectID}
            onChange={e => setSubjectID(e.target.value)}
            placeholder="e.g. user-123"
            className={inputClass}
          />
        </div>
        <div>
          <label className="block text-[13px] font-medium text-primary mb-1.5">Limit</label>
          <select value={limit} onChange={e => setLimit(+e.target.value)} className={`${inputClass} w-20`}>
            {LIMITS.map(l => <option key={l} value={l}>{l}</option>)}
          </select>
        </div>
        <button
          type="submit"
          disabled={isPending}
          className="bg-accent hover:bg-accent-hover active:bg-accent-active text-accent-text text-sm font-medium px-5 py-2 rounded-md border-0 cursor-pointer transition-colors duration-150 disabled:opacity-60 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:shadow-focus"
        >
          {isPending ? 'Fetching…' : 'Fetch'}
        </button>
      </form>

      {(debug.error || profile.error) && (
        <ErrorBanner message={debug.error?.message ?? profile.error?.message ?? 'Unknown error'} />
      )}

      {profile.data && (
        <div className="bg-surface border border-default rounded-lg p-5 mb-6">
          <h3 className="font-semibold m-0 mb-4 text-[11px] uppercase tracking-[0.06em] text-muted">
            Subject Profile
          </h3>
          <div className="grid grid-cols-3 gap-3 mb-4">
            {[
              { label: 'Total interactions', value: profile.data.interaction_count },
              { label: `Seen items (last ${profile.data.seen_items_days}d)`, value: profile.data.seen_items.length },
              {
                label: 'Sparse vector NNZ',
                value: profile.data.sparse_vector_nnz === -1 ? null : profile.data.sparse_vector_nnz,
                empty: 'not indexed',
              },
            ].map(({ label, value, empty }) => (
              <div key={label} className="flex flex-col p-4 bg-subtle border border-default rounded-lg">
                <span className="text-xs text-muted mb-1">{label}</span>
                {value != null
                  ? <span className="text-2xl font-bold text-primary tabular-nums -tracking-[0.02em]">{value}</span>
                  : <span className="text-sm text-muted mt-1">{empty}</span>}
              </div>
            ))}
          </div>

          {profile.data.seen_items.length > 0 && (
            <div>
              <div className="text-xs text-muted mb-2">
                Seen items (last {profile.data.seen_items_days} days — excluded from recommendations)
              </div>
              <div className="flex flex-wrap gap-1.5">
                {profile.data.seen_items.map(id => (
                  <code
                    key={id}
                    className="font-mono text-xs bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm font-medium"
                  >
                    {id}
                  </code>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {debug.data && (
        <div>
          <div className="mb-4 flex gap-4 flex-wrap items-center">
            <span className="text-sm text-secondary">
              Subject: <strong className="text-primary font-semibold">{debug.data.subject_id}</strong>
            </span>
            <span className="text-[11px] font-semibold uppercase tracking-[0.04em] bg-accent-subtle text-accent border border-accent/20 px-2 py-0.5 rounded-sm">
              {debug.data.source}
            </span>
            <span className="text-sm text-secondary tabular-nums">
              Total: <strong className="text-primary font-semibold">{debug.data.total}</strong>
            </span>
          </div>

          {debug.data.items.length === 0 ? (
            <p className="text-sm text-muted">No recommendations found for this subject.</p>
          ) : (
            <div className="bg-surface border border-default rounded-lg overflow-hidden">
              <table className="w-full border-collapse">
                <thead>
                  <tr className="bg-subtle border-b-2 border-default">
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Rank</th>
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Object ID</th>
                    <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Score</th>
                  </tr>
                </thead>
                <tbody>
                  {debug.data.items.map(item => (
                    <tr key={item.object_id} className="border-b border-default hover:bg-surface-raised">
                      <td className="px-4 py-3 text-sm text-muted tabular-nums">{item.rank}</td>
                      <td className="px-4 py-3 text-sm">
                        <code className="font-mono text-[12px] bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm font-medium">
                          {item.object_id}
                        </code>
                      </td>
                      <td className="px-4 py-3 text-sm text-primary font-mono tabular-nums">
                        {item.score.toFixed(4)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
