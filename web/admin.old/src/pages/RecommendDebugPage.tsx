import { useState } from 'react'
import { useRecommendDebug } from '../hooks/useRecommendDebug'
import { useSubjectProfile } from '../hooks/useSubjectProfile'
import ErrorBanner from '../components/ErrorBanner'
import { Badge, Button, CodeBadge, EmptyState, FormControl, MetricTile, PageShell, Panel, Select, Table, Thead, Th, Tbody, Tr, Td, TextInput, Toolbar } from '../components/ui'
import { useActiveNamespace } from '../context/useActiveNamespace'

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
    <PageShell title="Recommendation Debug">
      <Panel>
        <form onSubmit={handleSubmit}>
          <Toolbar>
            <FormControl label="Subject ID" htmlFor="debug-subject-id">
              <TextInput
                id="debug-subject-id"
                required
                value={subjectID}
                onChange={e => setSubjectID(e.target.value)}
                placeholder="e.g. user-123"
                className="w-full sm:w-64"
              />
            </FormControl>
            <FormControl label="Limit" htmlFor="debug-limit">
              <Select id="debug-limit" value={limit} onChange={e => setLimit(+e.target.value)} className="w-20">
                {LIMITS.map(l => <option key={l} value={l}>{l}</option>)}
              </Select>
            </FormControl>
            <Button
              type="submit"
              variant="primary"
              disabled={isPending}
              className="py-2"
            >
              {isPending ? 'Fetching...' : 'Fetch'}
            </Button>
          </Toolbar>
        </form>
      </Panel>

      {(debug.error || profile.error) && (
        <ErrorBanner message={debug.error?.message ?? profile.error?.message ?? 'Unknown error'} />
      )}

      {profile.data && (
        <Panel title="Subject Profile" className="mb-6">
          <div className="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
            {[
              { label: 'Total interactions', value: profile.data.interaction_count },
              { label: `Seen items (last ${profile.data.seen_items_days}d)`, value: profile.data.seen_items.length },
              {
                label: 'Sparse vector NNZ',
                value: profile.data.sparse_vector_nnz === -1 ? null : profile.data.sparse_vector_nnz,
                empty: 'not indexed',
              },
            ].map(({ label, value, empty }) => (
              <MetricTile
                key={label}
                label={label}
                value={value != null ? value : empty}
                valueClassName={value != null ? 'text-2xl' : 'text-sm'}
                className="bg-subtle"
              />
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
            <Badge tone="accent">
              {debug.data.source}
            </Badge>
            <span className="text-sm text-secondary tabular-nums">
              Total: <strong className="text-primary font-semibold">{debug.data.total}</strong>
            </span>
          </div>

          {debug.data.items.length === 0 ? (
            <EmptyState>No recommendations found for this subject.</EmptyState>
          ) : (
            <Panel bodyClassName="overflow-x-auto">
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
    </PageShell>
  )
}
