import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  Banner,
  Button,
  EmptyState,
  Pagination,
  Selector,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  TextInput,
} from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
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
 * index supports), not a substring search. The form still doubles as a direct
 * jump: submitting navigates straight to the typed id, which is how operators
 * who already know the id used to reach the inspector.
 */
export default function SubjectsListPage() {
  const { ns } = useParams<{ ns: string }>()
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<SubjectSort>('last_seen')
  const [page, setPage] = useState(0)

  const subjects = useSubjectsList(ns ?? null, {
    q: search || undefined,
    sort,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  })

  if (!ns) return null

  const openTyped = (e: FormEvent) => {
    e.preventDefault()
    const trimmed = search.trim()
    if (!trimmed) return
    navigate(`/ns/${encodeURIComponent(ns)}/subjects/${encodeURIComponent(trimmed)}`)
  }

  return (
    <PageContainer size="full">
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
        <form onSubmit={openTyped}>
          <Stack gap={4} direction="horizontal" align="center" wrap="wrap">
            <TextInput
              label="Search subjects"
              isLabelHidden
              value={search}
              onChange={(next) => {
                setSearch(next)
                setPage(0)
              }}
              hasClear
              size="sm"
              placeholder="subject_id starts with…"
            />
            <Button
              type="submit"
              size="sm"
              variant="secondary"
              
              isDisabled={search.trim() === ''} label="Open exact id" />
            <Selector
              size="sm"
              label="Sort"
              isLabelHidden
              value={sort}
              onChange={(next) => {
                setSort(next as SubjectSort)
                setPage(0)
              }}
              options={SORT_OPTIONS.map((o) => ({
                value: o.value,
                label: `sort: ${o.label}`,
              }))}
            />
            {subjects.data && (
              <span className="text-secondary text-sm ml-auto">page {page + 1}</span>
            )}
          </Stack>
        </form>

        {subjects.isLoading && <Skeleton className="h-48 w-full" />}

        {subjects.isError && (
          <Banner
            status="error"
            title="Failed to load subjects"
            description={subjects.error?.message ?? 'unknown error'}
          />
        )}

        {subjects.isSuccess && subjects.data.items.length === 0 && (
          <EmptyState
            title={search ? 'No subject id starts with that' : 'No subjects yet'}
            description={
              search
                ? 'Prefix match only — try a shorter prefix, or use "Open exact id" to jump straight to a subject.'
                : 'Subjects appear once events land for them. Inject a test event from the Events page to get started.'
            }
          />
        )}

        {subjects.isSuccess && subjects.data.items.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHeaderCell>Subject ID</TableHeaderCell>
                  <TableHeaderCell className="text-right" >Interactions</TableHeaderCell>
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
                    <TableCell  className="text-right tabular-nums">
                      {s.interaction_count.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-secondary text-sm">
                      {new Date(s.last_seen).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
        )}

        {subjects.data && subjects.data.total > PAGE_SIZE && (
          <Stack align="center" gap={4} direction="horizontal" justify="end">
            <Pagination
              page={page + 1}
              totalPages={Math.max(1, Math.ceil(subjects.data.total / PAGE_SIZE))}
              onChange={(p) => setPage(p - 1)}
            />
          </Stack>
        )}
      </Stack>
    </PageContainer>
  )
}
