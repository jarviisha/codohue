import { useState } from 'react'
import { useRecommendDebug } from '../hooks/useRecommendDebug'
import { useSubjectProfile } from '../hooks/useSubjectProfile'
import ErrorBanner from '../components/ErrorBanner'
import { Button, CodeBadge, EmptyState, PageHeader, Panel, Table, Thead, Th, Tbody, Tr, Td, inputClass } from '../components/ui'
import { useActiveNamespace } from '../context/NamespaceContext'

const LIMITS = [5, 10, 20, 50]

export default function RecommendDebugPage() {
  const { namespace } = useActiveNamespace()
  const debug = useRecommendDebug()
  const profile = useSubjectProfile()

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
      <PageHeader title="Recommendation Debug" />

      <form
        onSubmit={handleSubmit}
        className="bg-surface border border-default rounded-xl flex gap-4 flex-wrap items-end p-5 mb-6"
      >
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
        <Button
          type="submit"
          variant="primary"
          disabled={isPending}
          className="py-2"
        >
          {isPending ? 'Fetching…' : 'Fetch'}
        </Button>
      </form>

      {(debug.error || profile.error) && (
        <ErrorBanner message={debug.error?.message ?? profile.error?.message ?? 'Unknown error'} />
      )}

      {profile.data && (
        <Panel className="mb-6">
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
              <div key={label} className="flex flex-col p-4 bg-subtle border border-default rounded-xl">
                <span className="text-xs text-muted mb-1">{label}</span>
                {value != null
                  ? <span className="text-2xl font-bold text-primary tabular-nums tracking-[-0.02em]">{value}</span>
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
                  <CodeBadge key={id} className="text-xs">{id}</CodeBadge>
                ))}
              </div>
            </div>
          )}
        </Panel>
      )}

      {debug.data && (
        <div>
          <div className="mb-4 flex gap-4 flex-wrap items-center">
            <span className="text-sm text-secondary">
              Subject: <strong className="text-primary font-semibold">{debug.data.subject_id}</strong>
            </span>
            <span className="text-[11px] font-semibold uppercase tracking-[0.04em] bg-accent-subtle text-accent border border-accent/20 px-2 py-0.5 rounded-full">
              {debug.data.source}
            </span>
            <span className="text-sm text-secondary tabular-nums">
              Total: <strong className="text-primary font-semibold">{debug.data.total}</strong>
            </span>
          </div>

          {debug.data.items.length === 0 ? (
            <EmptyState>No recommendations found for this subject.</EmptyState>
          ) : (
            <Panel>
              <Table>
                <Thead>
                  <Th>Rank</Th>
                  <Th>Object ID</Th>
                  <Th>Score</Th>
                </Thead>
                <Tbody>
                  {debug.data.items.map(item => (
                    <Tr key={item.object_id} hoverable>
                      <Td muted mono>{item.rank}</Td>
                      <Td><CodeBadge>{item.object_id}</CodeBadge></Td>
                      <Td mono>{item.score.toFixed(4)}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </Panel>
          )}
        </div>
      )}
    </div>
  )
}
