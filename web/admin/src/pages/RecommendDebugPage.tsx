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
      <h2
        className="font-light text-[#061b31] m-0 mb-6"
        style={{ fontSize: '26px', letterSpacing: '-0.26px', lineHeight: 1.12 }}
      >
        Recommendation Debug
      </h2>

      <form
        onSubmit={handleSubmit}
        className="bg-white flex gap-4 flex-wrap items-end p-5 mb-6"
        style={{ border: '1px solid #e5edf5', borderRadius: '6px', boxShadow: 'rgba(23,23,23,0.06) 0px 3px 6px' }}
      >
        <div>
          <label className="block text-xs font-normal mb-1.5" style={{ color: '#273951' }}>Namespace</label>
          <select
            required
            value={namespace}
            onChange={e => setNamespace(e.target.value)}
            style={inputStyle}
            onFocus={e => { e.target.style.borderColor = '#533afd' }}
            onBlur={e => { e.target.style.borderColor = '#e5edf5' }}
          >
            <option value="">Select namespace</option>
            {nsData?.namespaces.map(ns => (
              <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="block text-xs font-normal mb-1.5" style={{ color: '#273951' }}>Subject ID</label>
          <input
            required
            value={subjectID}
            onChange={e => setSubjectID(e.target.value)}
            placeholder="e.g. user-123"
            style={inputStyle}
            onFocus={e => { e.target.style.borderColor = '#533afd' }}
            onBlur={e => { e.target.style.borderColor = '#e5edf5' }}
          />
        </div>
        <div>
          <label className="block text-xs font-normal mb-1.5" style={{ color: '#273951' }}>Limit</label>
          <select
            value={limit}
            onChange={e => setLimit(+e.target.value)}
            style={{ ...inputStyle, width: '80px' }}
            onFocus={e => { e.target.style.borderColor = '#533afd' }}
            onBlur={e => { e.target.style.borderColor = '#e5edf5' }}
          >
            {LIMITS.map(l => <option key={l} value={l}>{l}</option>)}
          </select>
        </div>
        <button
          type="submit"
          disabled={isPending}
          className="text-sm font-normal text-white transition-colors cursor-pointer"
          style={{
            background: isPending ? '#4434d4' : '#533afd',
            border: 'none',
            borderRadius: '4px',
            padding: '7px 18px',
            opacity: isPending ? 0.8 : 1,
            cursor: isPending ? 'not-allowed' : 'pointer',
          }}
          onMouseEnter={e => { if (!isPending) (e.currentTarget as HTMLElement).style.background = '#4434d4' }}
          onMouseLeave={e => { if (!isPending) (e.currentTarget as HTMLElement).style.background = '#533afd' }}
        >
          {isPending ? 'Fetching…' : 'Fetch'}
        </button>
      </form>

      {(debug.error || profile.error) && (
        <ErrorBanner message={debug.error?.message ?? profile.error?.message ?? 'Unknown error'} />
      )}

      {profile.data && (
        <div
          className="bg-white p-5 mb-6"
          style={{ border: '1px solid #e5edf5', borderRadius: '6px', boxShadow: 'rgba(23,23,23,0.06) 0px 3px 6px' }}
        >
          <h3
            className="font-normal text-[#061b31] m-0 mb-4"
            style={{ fontSize: '13px', letterSpacing: '0.06em', textTransform: 'uppercase', color: '#64748d' }}
          >
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
              <div
                key={label}
                className="flex flex-col p-4"
                style={{ background: '#fafbff', border: '1px solid #e5edf5', borderRadius: '5px' }}
              >
                <span className="text-xs text-[#64748d] font-light mb-1">{label}</span>
                {value != null
                  ? <span className="text-2xl font-light text-[#061b31] tabular-nums" style={{ letterSpacing: '-0.4px' }}>{value}</span>
                  : <span className="text-sm text-[#64748d] font-light mt-1">{empty}</span>}
              </div>
            ))}
          </div>

          {profile.data.seen_items.length > 0 && (
            <div>
              <div className="text-xs text-[#64748d] font-light mb-2">
                Seen items (last {profile.data.seen_items_days} days — excluded from recommendations)
              </div>
              <div className="flex flex-wrap gap-1.5">
                {profile.data.seen_items.map(id => (
                  <code
                    key={id}
                    className="text-xs"
                    style={{
                      fontFamily: "'Source Code Pro', monospace",
                      background: '#f5f6ff',
                      color: '#533afd',
                      padding: '1px 6px',
                      borderRadius: '3px',
                      fontWeight: 500,
                    }}
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
            <span className="text-sm text-[#64748d] font-light">
              Subject: <strong className="text-[#061b31] font-normal">{debug.data.subject_id}</strong>
            </span>
            <span
              className="text-xs font-normal px-2 py-0.5"
              style={{ background: '#f5f6ff', color: '#533afd', border: '1px solid rgba(83,58,253,0.2)', borderRadius: '4px' }}
            >
              {debug.data.source}
            </span>
            <span className="text-sm text-[#64748d] font-light tabular-nums">
              Total: <strong className="text-[#061b31] font-normal">{debug.data.total}</strong>
            </span>
          </div>

          {debug.data.items.length === 0 ? (
            <p className="text-sm text-[#64748d] font-light">No recommendations found for this subject.</p>
          ) : (
            <div
              className="bg-white overflow-hidden"
              style={{ border: '1px solid #e5edf5', borderRadius: '6px', boxShadow: 'rgba(23,23,23,0.06) 0px 3px 6px' }}
            >
              <table className="w-full border-collapse">
                <thead>
                  <tr style={{ borderBottom: '1px solid #e5edf5' }}>
                    <th style={thStyle}>Rank</th>
                    <th style={thStyle}>Object ID</th>
                    <th style={thStyle}>Score</th>
                  </tr>
                </thead>
                <tbody>
                  {debug.data.items.map(item => (
                    <tr key={item.object_id} style={{ borderBottom: '1px solid #e5edf5' }}>
                      <td style={{ ...tdStyle, color: '#64748d' }} className="tabular-nums">{item.rank}</td>
                      <td style={tdStyle}>
                        <code style={{ fontFamily: "'Source Code Pro', monospace", fontSize: '12px', background: '#f5f6ff', padding: '1px 6px', borderRadius: '3px', color: '#533afd', fontWeight: 500 }}>
                          {item.object_id}
                        </code>
                      </td>
                      <td style={{ ...tdStyle, fontFamily: "'Source Code Pro', monospace", fontSize: '12px' }} className="tabular-nums">
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

const inputStyle: React.CSSProperties = {
  padding: '7px 10px',
  border: '1px solid #e5edf5',
  borderRadius: '4px',
  fontSize: '13px',
  color: '#061b31',
  fontWeight: 300,
  background: '#fff',
  outline: 'none',
  transition: 'border-color 0.15s',
}

const thStyle: React.CSSProperties = {
  padding: '10px 16px',
  textAlign: 'left',
  fontSize: '12px',
  fontWeight: 400,
  color: '#64748d',
}

const tdStyle: React.CSSProperties = {
  padding: '10px 16px',
  fontSize: '13px',
  color: '#273951',
  fontWeight: 300,
}
