import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Button,
  Card,
  CardContent,
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
  SearchInput,
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
  eventsStreamPath,
  useEventsSummary,
  useInjectEvent,
  type EventStreamMessage,
  type EventSummary,
  type EventsSummaryResponse,
  type EventsSummaryWindow,
} from '@/services/events'
import { useServerStream } from '@/services/stream'
import PageHeader from '@/components/shell/PageHeader'
import TimeSeriesChart from '@/components/charts/TimeSeriesChart'
import NamespaceTag from '@/components/NamespaceTag'

const TAIL_CAP = 1000
const FLASH_MS = 1500
const KNOWN_ACTIONS = ['view', 'like', 'comment', 'share', 'skip'] as const
const WINDOWS: EventsSummaryWindow[] = ['1m', '5m', '1h']

/**
 * EventsPage owns the toolbar (action / subject filters, inject) and frames the
 * live tail + summary sidebar. The tail itself lives in <LiveTail>, keyed by
 * the stream URL so a filter change remounts it with a fresh buffer and a fresh
 * SSE subscription — no manual reset effect needed.
 */
export default function EventsPage() {
  const { ns } = useParams<{ ns: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const action = searchParams.get('action') ?? ''
  const subjectId = searchParams.get('subject_id') ?? ''

  const [draftSubject, setDraftSubject] = useState(subjectId)
  const [injectOpen, setInjectOpen] = useState(false)
  const [lastInjectedId, setLastInjectedId] = useState<number | null>(null)

  const setActionFilter = (next: string) => {
    const params = new URLSearchParams(searchParams)
    if (next) params.set('action', next)
    else params.delete('action')
    setSearchParams(params, { replace: true })
  }

  const applySubject = (e: FormEvent) => {
    e.preventDefault()
    const params = new URLSearchParams(searchParams)
    const trimmed = draftSubject.trim()
    if (trimmed) params.set('subject_id', trimmed)
    else params.delete('subject_id')
    setSearchParams(params, { replace: true })
  }

  const clearSubject = () => {
    setDraftSubject('')
    const params = new URLSearchParams(searchParams)
    params.delete('subject_id')
    setSearchParams(params, { replace: true })
  }

  if (!ns) return null

  const streamUrl =
    eventsStreamPath(ns, { action: action || undefined, subjectId: subjectId || undefined }) ?? ''

  return (
    <div className="px-6 py-6">
      <PageHeader>
        <Inline align="center" justify="between" className="w-full" wrap>
          <Stack gap="050">
            <h1 className="text-foreground text-xl font-semibold">
              Events · <NamespaceTag name={ns} />
            </h1>
            <p className="text-foreground-subtle text-sm">Live ingest tail · forward-only</p>
          </Stack>
          <Button size="sm" onClick={() => setInjectOpen(true)}>
            Inject test event
          </Button>
        </Inline>
      </PageHeader>

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_20rem] gap-6">
        <Stack>
          {lastInjectedId != null && (
            <Alert
              variant="success"
              title={`Injected event #${lastInjectedId}`}
              description="It flashes in the tail below as soon as ingest lands it."
            />
          )}

          <Inline align="center" wrap>
            <Button
              size="sm"
              variant={action === '' ? 'solid' : 'outline'}
              tone="neutral"
              onClick={() => setActionFilter('')}
            >
              all
            </Button>
            {KNOWN_ACTIONS.map((a) => (
              <Button
                key={a}
                size="sm"
                variant={action === a ? 'solid' : 'outline'}
                tone="neutral"
                onClick={() => setActionFilter(a)}
              >
                {a}
              </Button>
            ))}
            <form onSubmit={applySubject} className="max-w-xs w-full ml-auto">
              <SearchInput
                size="sm"
                value={draftSubject}
                onChange={(e) => setDraftSubject(e.target.value)}
                onClear={clearSubject}
                placeholder="Filter by subject id — Enter"
                aria-label="Filter tail by subject id"
              />
            </form>
          </Inline>

          <LiveTail
            key={streamUrl}
            namespace={ns}
            streamUrl={streamUrl}
            onInject={() => setInjectOpen(true)}
          />
        </Stack>

        <SummarySidebar namespace={ns} />
      </div>

      <Dialog open={injectOpen} onOpenChange={setInjectOpen} size="md">
        {injectOpen && (
          <InjectEventForm
            namespace={ns}
            onClose={() => setInjectOpen(false)}
            onInjected={(id) => setLastInjectedId(id)}
          />
        )}
      </Dialog>
    </div>
  )
}

/**
 * LiveTail subscribes to the events SSE stream and renders a 1000-row ring
 * buffer, newest first. Pausing buffers arrivals; resuming flushes them. New
 * rows flash briefly. A `dropped` frame from the server (client fell behind)
 * raises a warning banner.
 */
function LiveTail({
  namespace,
  streamUrl,
  onInject,
}: {
  namespace: string
  streamUrl: string
  onInject: () => void
}) {
  const [events, setEvents] = useState<EventSummary[]>([])
  const [paused, setPaused] = useState(false)
  const [flashIds, setFlashIds] = useState<Set<number>>(() => new Set())
  const [droppedCount, setDroppedCount] = useState(0)
  const [pendingCount, setPendingCount] = useState(0)

  const pausedRef = useRef(paused)
  useEffect(() => {
    pausedRef.current = paused
  }, [paused])
  const pendingRef = useRef<EventSummary[]>([])

  const flash = useCallback((id: number) => {
    setFlashIds((prev) => {
      const next = new Set(prev)
      next.add(id)
      return next
    })
    window.setTimeout(() => {
      setFlashIds((prev) => {
        if (!prev.has(id)) return prev
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    }, FLASH_MS)
  }, [])

  const { connected } = useServerStream(streamUrl || null, {
    event: (data: unknown) => {
      const e = data as EventStreamMessage
      if (pausedRef.current) {
        pendingRef.current = [e, ...pendingRef.current].slice(0, TAIL_CAP)
        setPendingCount(pendingRef.current.length)
        return
      }
      flash(e.id)
      setEvents((prev) => [e, ...prev].slice(0, TAIL_CAP))
    },
    dropped: (data: unknown) => {
      const d = data as { count?: number }
      setDroppedCount((c) => c + (d.count ?? 0))
    },
  })

  const resume = () => {
    const buffered = pendingRef.current
    pendingRef.current = []
    setPendingCount(0)
    setPaused(false)
    if (buffered.length > 0) {
      buffered.forEach((e) => flash(e.id))
      setEvents((prev) => [...buffered, ...prev].slice(0, TAIL_CAP))
    }
  }

  return (
    <Stack>
      <Inline align="center" justify="between" wrap>
        <Badge variant={connected ? 'success' : 'neutral'}>
          {connected ? 'streaming' : 'offline'}
        </Badge>
        <Button
          size="sm"
          variant="outline"
          tone="neutral"
          onClick={() => (paused ? resume() : setPaused(true))}
        >
          {paused ? `Resume${pendingCount > 0 ? ` (${pendingCount})` : ''}` : 'Pause'}
        </Button>
      </Inline>

      {droppedCount > 0 && (
        <Alert
          variant="warning"
          title={`${droppedCount} event${droppedCount === 1 ? '' : 's'} dropped`}
          description="The browser fell behind the ingest rate. Filter by action or subject to thin the stream."
        />
      )}

      {events.length === 0 ? (
        <EmptyState
          title={paused ? 'Tail paused' : 'Waiting for events'}
          description={
            paused
              ? 'Resume to start appending live events again.'
              : 'This is a forward-only tail — rows appear as ingest lands them. Inject a test event to see it flow through.'
          }
          actions={
            <Button size="sm" onClick={onInject}>
              Inject test event
            </Button>
          }
        />
      ) : (
        <TailTable namespace={namespace} items={events} flashIds={flashIds} />
      )}
    </Stack>
  )
}

function TailTable({
  namespace,
  items,
  flashIds,
}: {
  namespace: string
  items: EventSummary[]
  flashIds: Set<number>
}) {
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
            <TableRow
              key={e.id}
              className={
                flashIds.has(e.id) ? 'bg-background-selected transition-colors' : 'transition-colors'
              }
            >
              <TableCell className="text-foreground-subtle text-xs tabular-nums">
                {new Date(e.occurred_at).toLocaleTimeString()}
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

function SummarySidebar({ namespace }: { namespace: string }) {
  const [window, setWindow] = useState<EventsSummaryWindow>('1m')
  const summary = useEventsSummary(namespace, window)

  return (
    <Stack>
      <Inline align="center">
        {WINDOWS.map((w) => (
          <Button
            key={w}
            size="sm"
            variant={window === w ? 'solid' : 'outline'}
            tone="neutral"
            onClick={() => setWindow(w)}
          >
            {w}
          </Button>
        ))}
      </Inline>

      <Inline wrap>
        <SummaryTile
          label={`Events · ${window}`}
          value={(summary.data?.total ?? 0).toLocaleString()}
        />
        <SummaryTile label="Rate / s" value={(summary.data?.rate_per_second ?? 0).toFixed(2)} />
      </Inline>

      <ActionMix data={summary.data} />

      <Stack>
        <span className="text-foreground-subtle text-xs uppercase tracking-wide">Over time</span>
        {summary.data && summary.data.series.length > 0 ? (
          <TimeSeriesChart
            data={summary.data.series.map((b) => ({ ts: b.ts, count: b.count }))}
            series={[{ key: 'count', label: 'Events', color: 'var(--davinci-semantic-color-success)' }]}
            height={140}
          />
        ) : (
          <p className="text-foreground-subtle text-sm">No events in this window.</p>
        )}
      </Stack>
    </Stack>
  )
}

function SummaryTile({ label, value }: { label: string; value: string }) {
  return (
    <Card className="flex-1 min-w-30">
      <CardContent>
        <Stack>
          <span className="text-foreground-subtle text-xs uppercase tracking-wide">{label}</span>
          <span className="text-foreground text-xl font-semibold tabular-nums">{value}</span>
        </Stack>
      </CardContent>
    </Card>
  )
}

function ActionMix({ data }: { data: EventsSummaryResponse | undefined }) {
  if (!data || data.by_action.length === 0) {
    return null
  }
  const total = data.total || 1
  return (
    <Stack>
      <span className="text-foreground-subtle text-xs uppercase tracking-wide">Action mix</span>
      <Stack>
        {data.by_action.map((a) => {
          const pct = Math.round((a.count / total) * 100)
          return (
            <Stack key={a.action}>
              <Inline align="center" justify="between">
                <span className="text-foreground text-sm">{a.action}</span>
                <span className="text-foreground-subtle text-xs tabular-nums">
                  {a.count.toLocaleString()} · {pct}%
                </span>
              </Inline>
              <div className="h-1.5 w-full rounded-full bg-surface-sunken">
                <div className="h-1.5 rounded-full bg-success" style={{ width: `${pct}%` }} />
              </div>
            </Stack>
          )
        })}
      </Stack>
    </Stack>
  )
}

function InjectEventForm({
  namespace,
  onClose,
  onInjected,
}: {
  namespace: string
  onClose: () => void
  onInjected: (eventID: number) => void
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
        onSuccess: (res) => {
          onInjected(res.event_id)
          onClose()
        },
      },
    )
  }

  const canSubmit = !inject.isPending && subjectId.trim() !== '' && objectId.trim() !== ''

  return (
    <form onSubmit={onSubmit} className="contents">
      <DialogHeader>
        <DialogTitle>
          Inject test event · <NamespaceTag name={namespace} />
        </DialogTitle>
        <DialogDescription>
          Proxied through the admin event injection endpoint. Lands in the same events table the
          ingest worker writes to, and flashes in the live tail as it arrives.
        </DialogDescription>
      </DialogHeader>
      <DialogContent>
        <Stack>
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
        <Inline justify="end">
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
