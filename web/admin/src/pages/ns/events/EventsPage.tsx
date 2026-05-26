import { useState, type FormEvent } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Button,
  Container,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  EmptyState,
  FormField,
  Inline,
  Input,
  Pagination,
  SearchInput,
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
import {
  useInjectEvent,
  useRecentEvents,
  type EventSummary,
} from '@/services/events'
import PageHeader from '@/components/shell/PageHeader'

const PAGE_SIZE = 50

/**
 * EventsPage tails the events table for one namespace. Operators use it to
 * verify ingest is landing, debug a quiet subject, or inject a test event
 * when wiring a new client.
 *
 *   - The list auto-refreshes every 10s (matches the service-layer poll).
 *   - `?subject_id=` filter survives URL refresh so deep links from the
 *     subject inspector remain bookmark-able.
 *   - "Inject event" toggles an inline form; on success it refetches the
 *     list so the new row appears at the top.
 */
export default function EventsPage() {
  const { ns } = useParams<{ ns: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const subjectId = searchParams.get('subject_id') ?? ''
  const offset = Number(searchParams.get('offset') ?? '0') || 0

  const [draftSubjectId, setDraftSubjectId] = useState(subjectId)
  const [injectOpen, setInjectOpen] = useState(false)

  const events = useRecentEvents(ns ?? null, {
    subjectId: subjectId || undefined,
    limit: PAGE_SIZE,
    offset,
  })

  if (!ns) return null

  const applyFilter = (e: FormEvent) => {
    e.preventDefault()
    const next = new URLSearchParams(searchParams)
    const trimmed = draftSubjectId.trim()
    if (trimmed) {
      next.set('subject_id', trimmed)
    } else {
      next.delete('subject_id')
    }
    next.delete('offset')
    setSearchParams(next, { replace: true })
  }

  const clearFilter = () => {
    setDraftSubjectId('')
    const next = new URLSearchParams(searchParams)
    next.delete('subject_id')
    next.delete('offset')
    setSearchParams(next, { replace: true })
  }

  const setOffset = (next: number) => {
    const params = new URLSearchParams(searchParams)
    if (next > 0) {
      params.set('offset', String(next))
    } else {
      params.delete('offset')
    }
    setSearchParams(params, { replace: true })
  }

  const total = events.data?.total ?? 0
  const items = events.data?.items ?? []

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Inline gap="200" align="center" justify="between" className="w-full" wrap>
          <Stack gap="025">
            <h1 className="text-foreground text-xl font-semibold">Events · {ns}</h1>
            <p className="text-foreground-subtle text-sm">
              Recent ingest tail. Auto-refreshes every 10 seconds.
            </p>
          </Stack>
          <Button size="sm" onClick={() => setInjectOpen(true)}>
            Inject test event
          </Button>
        </Inline>
      </PageHeader>

      <Stack gap="300">
        <Inline gap="200" align="center" justify="between" wrap>
          <form onSubmit={applyFilter} className="max-w-sm w-full">
            <SearchInput
              size="sm"
              value={draftSubjectId}
              onChange={(e) => setDraftSubjectId(e.target.value)}
              onClear={clearFilter}
              placeholder="Filter by subject id — press Enter"
              aria-label="Filter events by subject id"
            />
          </form>
          <span className="text-foreground-subtle text-xs tabular-nums">
            {events.isLoading
              ? '…'
              : `${total.toLocaleString()} event${total === 1 ? '' : 's'}${subjectId ? ` · subject ${subjectId}` : ''}`}
          </span>
        </Inline>

        {events.isError && (
          <Alert
            variant="danger"
            title="Could not load events"
            description={events.error?.message ?? 'unknown error'}
          />
        )}

        {events.isLoading ? (
          <Skeleton className="h-60 w-full" />
        ) : items.length === 0 ? (
          <EmptyState
            title={subjectId ? `No events for subject "${subjectId}"` : 'No events yet'}
            description={
              subjectId
                ? 'Either this subject id has never been seen, or every event is outside the current page. Clear the filter to scan the full namespace.'
                : 'Once ingest lands the first event for this namespace, it shows up here within seconds.'
            }
            actions={
              subjectId ? (
                <Button size="sm" variant="outline" tone="neutral" onClick={clearFilter}>
                  Clear filter
                </Button>
              ) : (
                <Button size="sm" onClick={() => setInjectOpen(true)}>
                  Inject test event
                </Button>
              )
            }
          />
        ) : (
          <Stack gap="100">
            <EventsTable namespace={ns} items={items} />
            <Pagination
              page={Math.floor(offset / PAGE_SIZE) + 1}
              pageCount={Math.max(1, Math.ceil(total / PAGE_SIZE))}
              onPageChange={(page) => setOffset((page - 1) * PAGE_SIZE)}
            />
          </Stack>
        )}
      </Stack>

      <InjectEventDialog
        namespace={ns}
        open={injectOpen}
        onOpenChange={setInjectOpen}
      />
    </Container>
  )
}

function EventsTable({ namespace, items }: { namespace: string; items: EventSummary[] }) {
  return (
    <TableContainer>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Occurred</TableHead>
            <TableHead>Subject</TableHead>
            <TableHead>Object</TableHead>
            <TableHead>Action</TableHead>
            <TableHead align="right">Weight</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((e) => (
            <TableRow key={e.id}>
              <TableCell className="text-foreground-subtle text-xs tabular-nums">
                {new Date(e.occurred_at).toLocaleString()}
              </TableCell>
              <TableCell>
                <Link
                  to={`/ns/${encodeURIComponent(namespace)}/subjects/${encodeURIComponent(e.subject_id)}`}
                  className="text-foreground text-sm font-medium"
                >
                  <code>{e.subject_id}</code>
                </Link>
              </TableCell>
              <TableCell>
                <code className="text-foreground-subtle text-xs">{e.object_id}</code>
              </TableCell>
              <TableCell>
                <Badge variant="neutral">{e.action}</Badge>
              </TableCell>
              <TableCell align="right" className="tabular-nums">
                {e.weight.toFixed(2)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

function InjectEventDialog({
  namespace,
  open,
  onOpenChange,
}: {
  namespace: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} size="md">
      {open && (
        <InjectEventForm namespace={namespace} onClose={() => onOpenChange(false)} />
      )}
    </Dialog>
  )
}

function InjectEventForm({
  namespace,
  onClose,
}: {
  namespace: string
  onClose: () => void
}) {
  const inject = useInjectEvent(namespace)
  const [subjectId, setSubjectId] = useState('')
  const [objectId, setObjectId] = useState('')
  const [action, setAction] = useState('view')

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    inject.mutate(
      { subject_id: subjectId.trim(), object_id: objectId.trim(), action },
      {
        onSuccess: () => onClose(),
      },
    )
  }

  const canSubmit =
    !inject.isPending && subjectId.trim() !== '' && objectId.trim() !== ''

  return (
    <form onSubmit={onSubmit}>
      <DialogHeader>
        <DialogTitle>Inject test event · {namespace}</DialogTitle>
        <DialogDescription>
          Proxied through the admin event injection endpoint. Lands in the same events table the
          ingest worker writes to — useful for wiring up a new client or unblocking a quiet subject.
        </DialogDescription>
      </DialogHeader>
      <DialogContent>
        <Stack gap="200">
          {inject.error && (
            <Alert variant="danger" title="Inject failed" description={inject.error.message} />
          )}
          <FormField label="Subject ID" required>
            <Input
              value={subjectId}
              onChange={(e) => setSubjectId(e.target.value)}
              placeholder="user-42"
              autoFocus
            />
          </FormField>
          <FormField label="Object ID" required>
            <Input
              value={objectId}
              onChange={(e) => setObjectId(e.target.value)}
              placeholder="item-100"
            />
          </FormField>
          <FormField label="Action" helpText="Matches an entry in the namespace action_weights map.">
            <Input value={action} onChange={(e) => setAction(e.target.value)} />
          </FormField>
        </Stack>
      </DialogContent>
      <DialogFooter>
        <Inline gap="100" justify="end">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!canSubmit}>
            {inject.isPending ? 'Injecting…' : 'Inject'}
          </Button>
        </Inline>
      </DialogFooter>
    </form>
  )
}
