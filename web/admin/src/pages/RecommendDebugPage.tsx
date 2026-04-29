import { useState } from 'react'
import { useRecommendDebug } from '../hooks/useRecommendDebug'
import { useSubjectProfile } from '../hooks/useSubjectProfile'
import { useNamespaceList } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

const LIMITS = [5, 10, 20, 50]

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
      <h2 className="mt-0 mb-4 text-xl font-semibold text-gray-800">Recommendation Debug</h2>

      <form
        onSubmit={handleSubmit}
        className="bg-white border border-gray-200 rounded-lg p-4 mb-6 flex gap-3 flex-wrap items-end"
      >
        <div>
          <label className={label}>Namespace</label>
          <select required value={namespace} onChange={e => setNamespace(e.target.value)} className={input}>
            <option value="">Select namespace</option>
            {nsData?.namespaces.map(ns => (
              <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
            ))}
          </select>
        </div>
        <div>
          <label className={label}>Subject ID</label>
          <input
            required
            value={subjectID}
            onChange={e => setSubjectID(e.target.value)}
            placeholder="e.g. user-123"
            className={input}
          />
        </div>
        <div>
          <label className={label}>Limit</label>
          <select value={limit} onChange={e => setLimit(+e.target.value)} className={`${input} w-20`}>
            {LIMITS.map(l => <option key={l} value={l}>{l}</option>)}
          </select>
        </div>
        <button
          type="submit"
          disabled={isPending}
          className={`px-4 py-2 bg-blue-600 text-white border-none rounded text-sm font-medium ${
            isPending ? 'opacity-70 cursor-not-allowed' : 'cursor-pointer hover:bg-blue-700'
          }`}
        >
          {isPending ? 'Fetching…' : 'Fetch'}
        </button>
      </form>

      {(debug.error || profile.error) && (
        <ErrorBanner message={debug.error?.message ?? profile.error?.message ?? 'Unknown error'} />
      )}

      {profile.data && (
        <div className="bg-white border border-gray-200 rounded-lg p-4 mb-6">
          <h3 className="mt-0 mb-3 text-sm font-semibold text-gray-700 uppercase tracking-wide">Subject Profile</h3>
          <div className="grid grid-cols-3 gap-4 mb-4">
            <div className="bg-gray-50 rounded p-3">
              <div className="text-xs text-gray-500 mb-1">Total interactions</div>
              <div className="text-2xl font-bold text-gray-800">{profile.data.interaction_count}</div>
            </div>
            <div className="bg-gray-50 rounded p-3">
              <div className="text-xs text-gray-500 mb-1">Seen items (last {profile.data.seen_items_days}d)</div>
              <div className="text-2xl font-bold text-gray-800">{profile.data.seen_items.length}</div>
            </div>
            <div className="bg-gray-50 rounded p-3">
              <div className="text-xs text-gray-500 mb-1">Sparse vector NNZ</div>
              <div className="text-2xl font-bold text-gray-800">
                {profile.data.sparse_vector_nnz === -1
                  ? <span className="text-base text-gray-400">not indexed</span>
                  : profile.data.sparse_vector_nnz}
              </div>
            </div>
          </div>

          {profile.data.seen_items.length > 0 && (
            <div>
              <div className="text-xs text-gray-500 mb-2">
                Seen items (last {profile.data.seen_items_days} days — excluded from recommendations)
              </div>
              <div className="flex flex-wrap gap-1.5">
                {profile.data.seen_items.map(id => (
                  <code key={id} className="text-xs bg-gray-100 text-gray-700 px-1.5 py-0.5 rounded font-mono">
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
          <div className="mb-3 flex gap-4 text-sm text-gray-600">
            <span>Subject: <strong className="text-gray-800">{debug.data.subject_id}</strong></span>
            <span>Source: <strong className="bg-blue-50 text-blue-600 px-1.5 py-0.5 rounded text-xs">{debug.data.source}</strong></span>
            <span>Total: <strong className="text-gray-800">{debug.data.total}</strong></span>
          </div>

          {debug.data.items.length === 0 ? (
            <p className="text-gray-400">No recommendations found for this subject.</p>
          ) : (
            <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
              <table className="w-full border-collapse">
                <thead>
                  <tr className="bg-gray-50 border-b border-gray-200">
                    <th className={th}>Rank</th>
                    <th className={th}>Object ID</th>
                    <th className={th}>Score</th>
                  </tr>
                </thead>
                <tbody>
                  {debug.data.items.map(item => (
                    <tr key={item.object_id} className="border-b border-gray-100">
                      <td className={td}>{item.rank}</td>
                      <td className={td}><code className="font-mono text-sm bg-gray-100 px-1.5 py-0.5 rounded">{item.object_id}</code></td>
                      <td className={td}>{item.score.toFixed(4)}</td>
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

const label = 'block text-xs text-gray-500 mb-1'
const input = 'px-2.5 py-1.5 border border-gray-300 rounded text-sm'
const th = 'px-4 py-2.5 text-left text-sm font-semibold text-gray-500'
const td = 'px-4 py-2.5 text-sm text-gray-700'
