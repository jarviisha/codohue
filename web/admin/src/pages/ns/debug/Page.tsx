import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import {
  Button,
  CodeBlock,
  EmptyState,
  Field,
  Form,
  Input,
  KeyValueList,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  Panel,
  Select,
  Switch,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
  useRegisterCommand,
} from '@/components/ui'
import {
  useRecommendDebug,
  useSubjectProfile,
} from '@/services/recommend'
import { formatNumber, formatTimestamp } from '@/utils/format'

const DEFAULT_LIMIT = 10

interface QueryParams {
  subjectID: string
  limit: number
  debug: boolean
}

const EMPTY_QUERY: QueryParams = {
  subjectID: '',
  limit: DEFAULT_LIMIT,
  debug: true,
}

// Operator-only debug view for the recommend pipeline. The form drives a
// pair of queries (subject profile + recommendations) gated on subject_id;
// the debug envelope plus the raw JSON envelope are surfaced so operators
// can audit scoring composition without grepping logs.
export default function DebugPage() {
  const { name = '' } = useParams<{ name: string }>()
  const [form, setForm] = useState<QueryParams>(EMPTY_QUERY)
  const [submitted, setSubmitted] = useState<QueryParams | null>(null)

  const recommend = useRecommendDebug(
    {
      namespace: name,
      subject_id: submitted?.subjectID ?? '',
      limit: submitted?.limit,
      debug: submitted?.debug,
    },
    Boolean(submitted),
  )

  const profile = useSubjectProfile(
    {
      namespace: name,
      subject_id: submitted?.subjectID ?? '',
    },
    Boolean(submitted),
  )

  useRegisterCommand(
    `ns.${name}.debug.refresh`,
    `Refresh ${name} recommend debug`,
    () => {
      if (submitted) {
        void recommend.refetch()
        void profile.refetch()
      }
    },
    name,
  )

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const trimmed = form.subjectID.trim()
    if (!trimmed) {
      setSubmitted(null)
      return
    }
    setSubmitted({ ...form, subjectID: trimmed })
  }

  const data = recommend.data
  const profileData = profile.data

  return (
    <PageShell>
      <PageHeader title="recommend debug" />

      <Panel title="query">
        <Form onSubmit={submit}>
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-64 flex-1">
              <Field label="subject_id" htmlFor="debug-subject-id" required>
                <Input
                  id="debug-subject-id"
                  value={form.subjectID}
                  placeholder="user_19283"
                  onChange={(event) => setForm((f) => ({ ...f, subjectID: event.target.value }))}
                />
              </Field>
            </div>
            <Field label="limit" htmlFor="debug-limit">
              <Select
                id="debug-limit"
                value={String(form.limit)}
                onChange={(event) => setForm((f) => ({ ...f, limit: Number(event.target.value) }))}
              >
                {[5, 10, 25, 50, 100].map((value) => (
                  <option key={value} value={value}>{value}</option>
                ))}
              </Select>
            </Field>
            <Field label="debug" htmlFor="debug-toggle">
              <div className="flex items-center gap-2 pt-1">
                <Switch
                  id="debug-toggle"
                  checked={form.debug}
                  onChange={(next) => setForm((f) => ({ ...f, debug: next }))}
                  ariaLabel="Include debug envelope"
                />
                <span className="font-mono text-xs text-secondary">
                  include envelope
                </span>
              </div>
            </Field>
            <Button type="submit" variant="primary">
              Run
            </Button>
          </div>
        </Form>
      </Panel>

      {recommend.isError ? (
        <Notice tone="fail" title="Failed to load recommendations">
          {(recommend.error as Error)?.message ?? 'Unable to load recommendations.'}
        </Notice>
      ) : null}
      {profile.isError ? (
        <Notice tone="fail" title="Failed to load subject profile">
          {(profile.error as Error)?.message ?? 'Unable to load subject profile.'}
        </Notice>
      ) : null}

      <Panel
        title="subject profile"
        busy={profile.isFetching && !profile.isLoading}
      >
        {!submitted ? (
          <EmptyState
            title="Enter a subject_id"
            description="Run the form above to inspect a subject's profile and recommendations."
          />
        ) : profile.isLoading ? (
          <LoadingState rows={4} label="loading subject profile" />
        ) : profileData ? (
          <KeyValueList
            rows={[
              { label: 'subject_id', value: profileData.subject_id },
              { label: 'interaction_count', value: formatNumber(profileData.interaction_count) },
              {
                label: 'sparse_vector_nnz',
                value:
                  profileData.sparse_vector_nnz < 0
                    ? 'not indexed'
                    : formatNumber(profileData.sparse_vector_nnz),
              },
              {
                label: 'seen_items',
                value: `${formatNumber(profileData.seen_items.length)} (${profileData.seen_items_days}d window)`,
              },
            ]}
          />
        ) : null}
      </Panel>

      {submitted?.debug && data?.debug ? (
        <Panel title="debug envelope">
          <KeyValueList
            rows={[
              { label: 'source', value: data.source },
              { label: 'alpha', value: data.debug.alpha.toFixed(3) },
              { label: 'sparse_nnz', value: formatNumber(data.debug.sparse_nnz) },
              { label: 'dense_score', value: data.debug.dense_score.toFixed(4) },
              { label: 'seen_items_count', value: formatNumber(data.debug.seen_items_count) },
              { label: 'interaction_count', value: formatNumber(data.debug.interaction_count) },
              { label: 'generated_at', value: formatTimestamp(data.generated_at) },
            ]}
          />
        </Panel>
      ) : null}

      <Panel
        title="recommendations"
        busy={recommend.isFetching && !recommend.isLoading}
        actions={
          data ? (
            <span className="font-mono text-xs text-muted">source · {data.source}</span>
          ) : null
        }
      >
        {!submitted ? (
          <EmptyState
            title="No query yet"
            description="Submit the form above to fetch recommendations."
          />
        ) : recommend.isLoading ? (
          <LoadingState rows={6} label="loading recommendations" />
        ) : !data || data.items.length === 0 ? (
          <EmptyState
            title="No items returned"
            description="The subject may have no recommendations under the current scoring."
          />
        ) : (
          <Table>
            <Thead>
              <Tr>
                <Th align="right">rank</Th>
                <Th>object_id</Th>
                <Th align="right">score</Th>
              </Tr>
            </Thead>
            <Tbody>
              {data.items.map((item) => (
                <Tr key={item.object_id}>
                  <Td mono align="right">{formatNumber(item.rank)}</Td>
                  <Td mono>{item.object_id}</Td>
                  <Td mono align="right">{item.score.toFixed(4)}</Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </Panel>

      {submitted && data ? (
        <Panel title="raw response">
          <CodeBlock language="json" copyable maxHeight="20rem">
            {JSON.stringify(data, null, 2)}
          </CodeBlock>
        </Panel>
      ) : null}
    </PageShell>
  )
}
