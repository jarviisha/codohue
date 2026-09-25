import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  Button,
  EmptyState,
  Pagination,
  Selector,
  Skeleton,
  Stack,
  Table,
  proportional,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from '@astryxdesign/core'
import QueryFeedback from '@/components/QueryFeedback'
import ListSearch from '@/components/ListSearch'
import { readPage } from '@/services/operatorUx'
import { useSubjectsList, type SubjectSort } from '@/services/subjects'
import PageHeader from '@/components/shell/PageHeader'

const PAGE_SIZE = 25

const SORT_OPTIONS: Array<{ value: SubjectSort; label: string }> = [
  { value: 'last_seen', label: 'recently active' },
  { value: 'interactions', label: 'most interactions' },
  { value: 'subject_id', label: 'subject id' },
]

/**
 * SubjectsListPage browses the subjects that have events in a namespace.
 *
 * Subjects aren't a stored resource — each row is an aggregate over the events
 * table, so the search box is a subject_id *prefix* filter (the shape the
 * index supports), not a substring search. Apply filter stores the prefix in the URL. Open exact ID is a separate
 * action for operators who already know the subject they want to inspect.
 */
export default function SubjectsListPage() {
  const { ns } = useParams<{ ns: string }>()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const search = searchParams.get('q') ?? ''
  const rawSort = searchParams.get('sort') ?? 'last_seen'
  const sort: SubjectSort = SORT_OPTIONS.some((option) => option.value === rawSort)
    ? (rawSort as SubjectSort)
    : 'last_seen'
  const page = readPage(searchParams.get('page'))

  const updateFilter = (key: string, value: string) => {
    const params = new URLSearchParams(searchParams)
    if (value) params.set(key, value)
    else params.delete(key)
    if (key !== 'page') params.delete('page')
    setSearchParams(params)
  }

  const subjects = useSubjectsList(ns ?? null, {
    q: search || undefined,
    sort,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  })

  if (!ns) return null

  return (
    <>
      <PageHeader>
        <Stack gap={1}>
          <h1 className="text-primary text-xl font-semibold">Subjects</h1>
          <p className="text-secondary text-sm">
            {subjects.data
              ? `${subjects.data.total.toLocaleString()} subjects with events. Click one to inspect its profile and recommendations.`
              : 'Subjects seen in this namespace, derived from the events table.'}
          </p>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        <Stack gap={4} direction="horizontal" align="end" wrap="wrap">
          <ListSearch
            key={`${ns}:${search}`}
            value={search}
            label="Search subjects"
            description="Subject ID starts with this text."
            onApply={(value) => updateFilter('q', value)}
            onOpen={(value) =>
              navigate(`/ns/${encodeURIComponent(ns)}/subjects/${encodeURIComponent(value)}`)
            }
          />
          <Selector
            size="sm"
            label="Sort"
            value={sort}
            onChange={(next) => {
              updateFilter('sort', next)
            }}
            options={SORT_OPTIONS.map((o) => ({
              value: o.value,
              label: `sort: ${o.label}`,
            }))}
          />
          {subjects.data && <span className="text-secondary text-sm ml-auto">page {page + 1}</span>}
        </Stack>

        <QueryFeedback query={subjects} label="Subjects" />
        {subjects.isLoading && <Skeleton height={192} />}

        {subjects.data && subjects.data.items.length === 0 && (
          <EmptyState
            title={search ? 'No subject id starts with that' : 'No subjects yet'}
            description={
              search
                ? 'Prefix match only — try a shorter prefix, or use "Open exact id" to jump straight to a subject.'
                : 'Subjects appear once events land for them. Inject a test event from the Events page to get started.'
            }
          />
        )}

        {subjects.data && subjects.data.items.length > 0 && (
          <Stack className="min-w-0">
            <Table
              aria-label="Subjects"
              columns={['Subject ID', 'Interactions', 'Last seen'].map((key) => ({
                key,
                header: key,
                width: proportional(1),
              }))}
            >
              <TableHeader>
                <TableRow>
                  <TableHeaderCell>Subject ID</TableHeaderCell>
                  <TableHeaderCell className="text-right">Interactions</TableHeaderCell>
                  <TableHeaderCell>Last seen</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {subjects.data.items.map((s) => (
                  <TableRow key={s.subject_id}>
                    <TableCell>
                      <Link
                        to={`/ns/${encodeURIComponent(ns)}/subjects/${encodeURIComponent(s.subject_id)}`}
                        className="text-primary font-medium"
                      >
                        {s.subject_id}
                      </Link>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {s.interaction_count.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-secondary text-sm">
                      {new Date(s.last_seen).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Stack>
        )}

        {subjects.data && page > 0 && subjects.data.items.length === 0 && (
          <Button label="Return to first page" onClick={() => updateFilter('page', '')} />
        )}
        {subjects.data && subjects.data.total > PAGE_SIZE && (
          <Stack align="center" gap={4} direction="horizontal" justify="end">
            <Pagination
              page={page + 1}
              totalPages={Math.max(1, Math.ceil(subjects.data.total / PAGE_SIZE))}
              onChange={(p) => updateFilter('page', String(p))}
            />
          </Stack>
        )}
      </Stack>
    </>
  )
}
