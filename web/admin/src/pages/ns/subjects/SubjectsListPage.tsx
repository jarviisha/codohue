import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  Alert,
  Button,
  Container,
  EmptyState,
  Inline,
  Pagination,
  SearchInput,
  Select,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from '@jarviisha/davinci-react-ui'
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
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Stack gap="050">
          <h1 className="text-foreground text-xl font-semibold">Subjects</h1>
          <p className="text-foreground-subtle text-sm">
            {subjects.data
              ? `${subjects.data.total.toLocaleString()} subjects with events. Click one to inspect its profile and recommendations.`
              : 'Subjects seen in this namespace, derived from the events table.'}
          </p>
        </Stack>
      </PageHeader>

      <Stack>
        <form onSubmit={openTyped}>
          <Inline align="center" wrap>
            <SearchInput
              size="sm"
              placeholder="subject_id starts with…"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                setPage(0)
              }}
              onClear={() => {
                setSearch('')
                setPage(0)
              }}
            />
            <Button
              type="submit"
              size="sm"
              variant="outline"
              tone="neutral"
              disabled={search.trim() === ''}
            >
              Open exact id
            </Button>
            <Select
              size="sm"
              value={sort}
              onChange={(e) => {
                setSort(e.target.value as SubjectSort)
                setPage(0)
              }}
            >
              {SORT_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  sort: {o.label}
                </option>
              ))}
            </Select>
            {subjects.data && (
              <span className="text-foreground-subtle text-sm ml-auto">page {page + 1}</span>
            )}
          </Inline>
        </form>

        {subjects.isLoading && <Skeleton className="h-48 w-full" />}

        {subjects.isError && (
          <Alert
            variant="danger"
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
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Subject ID</TableHead>
                  <TableHead align="right">Interactions</TableHead>
                  <TableHead>Last seen</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {subjects.data.items.map((s) => (
                  <TableRow key={s.subject_id}>
                    <TableCell>
                      <Link
                        to={`/ns/${encodeURIComponent(ns)}/subjects/${encodeURIComponent(s.subject_id)}`}
                        className="text-foreground font-medium"
                      >
                        {s.subject_id}
                      </Link>
                    </TableCell>
                    <TableCell align="right" className="tabular-nums">
                      {s.interaction_count.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-foreground-subtle text-sm">
                      {new Date(s.last_seen).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}

        {subjects.data && subjects.data.total > PAGE_SIZE && (
          <Inline justify="end">
            <Pagination
              page={page + 1}
              pageCount={Math.max(1, Math.ceil(subjects.data.total / PAGE_SIZE))}
              onPageChange={(p) => setPage(p - 1)}
            />
          </Inline>
        )}
      </Stack>
    </Container>
  )
}
