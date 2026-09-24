import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import {
  Token,
  Skeleton,
  SegmentedControl,
  SegmentedControlItem,
  Grid,
  ProgressBar,
  Banner,
  Button,
  Card,
  Dialog,
  DialogHeader,
  EmptyState,
  Layout,
  LayoutContent,
  LayoutFooter,
  Stack,
  Table,
  proportional,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  TextInput,
} from '@astryxdesign/core'
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
import PageContainer from '@/components/PageContainer'
import QueryFeedback from '@/components/QueryFeedback'
import PageHeader from '@/components/shell/PageHeader'
import TimeSeriesChart from '@/components/charts/TimeSeriesChart'

/** Events kept in memory for scrollback. Not the number rendered. */
const TAIL_CAP = 1000
/**
 * Rows committed to the DOM at once. Roughly a tall viewport plus overscan.
 * The cap is what keeps a populated tail off the main thread: rendering the
 * full 1000-row history took hundreds of milliseconds and starved input.
 */
const WINDOW_SIZE = 60
const FLASH_MS = 1500
/**
 * Arrivals are collected for this long and applied as one update. At any
 * ingest rate the tail costs at most ~10 renders per second instead of one
 * per event.
 */
const FLUSH_MS = 100
/** How often expired flashes are swept. One timer for the whole table. */
const FLASH_SWEEP_MS = 250
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
  const [draftAction, setDraftAction] = useState(action)
  const [injectOpen, setInjectOpen] = useState(false)
  const [lastInjectedId, setLastInjectedId] = useState<number | null>(null)

  const setActionFilter = (next: string) => {
    const params = new URLSearchParams(searchParams)
    if (next) params.set('action', next)
    else params.delete('action')
    setSearchParams(params, { replace: true })
  }

  // Free text, not a fixed vocabulary: action names are whatever the namespace
  // sends (the bundled demo uses VIEW / LIKE / CART / PURCHASE) and the server
  // compares them exactly, case included. A hard-coded lowercase chip row
  // matched nothing in such a namespace and hid the actions it did use.
  const applyAction = (e: FormEvent) => {
    e.preventDefault()
    setActionFilter(draftAction.trim())
  }

  const clearAction = () => {
    setDraftAction('')
    setActionFilter('')
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
    <PageContainer size="full">
      <PageHeader>
        <Stack
          gap={4}
          direction="horizontal"
          align="center"
          justify="between"
          className="w-full"
          wrap="wrap"
        >
          <Stack gap={1}>
            <h1 className="text-primary text-xl font-semibold">Events</h1>
            <p className="text-secondary text-sm">Live ingest tail, forward-only</p>
          </Stack>
          <Button size="sm" onClick={() => setInjectOpen(true)} label="Inject test event" />
        </Stack>
      </PageHeader>

      <Grid columns={{ minWidth: 320, max: 2 }} gap={6}>
        <Stack gap={6}>
          {lastInjectedId != null && (
            <Banner
              status="success"
              title={`Injected event #${lastInjectedId}`}
              description="It flashes in the tail below as soon as ingest lands it."
            />
          )}

          <Stack gap={4} direction="horizontal" align="center" wrap="wrap">
            <form onSubmit={applyAction} className="max-w-xs w-full">
              <TextInput
                value={draftAction}
                onChange={(next) => (next === '' ? clearAction() : setDraftAction(next))}
                hasClear
                label="Filter tail by action"
                isLabelHidden
                size="sm"
                placeholder="Filter by action — exact, case-sensitive"
              />
            </form>
            {action && <Token color="blue" label={`action = ${action}`} />}
            <form onSubmit={applySubject} className="max-w-xs w-full ml-auto">
              <TextInput
                value={draftSubject}
                onChange={(next) => (next === '' ? clearSubject() : setDraftSubject(next))}
                hasClear
                label="Filter tail by subject id"
                isLabelHidden
                size="sm"
                placeholder="Filter by subject id — Enter"
              />
            </form>
          </Stack>

          <LiveTail
            key={streamUrl}
            namespace={ns}
            streamUrl={streamUrl}
            onInject={() => setInjectOpen(true)}
          />
        </Stack>

        <SummarySidebar namespace={ns} />
      </Grid>

      <Dialog isOpen={injectOpen} onOpenChange={setInjectOpen} width={560} purpose="form">
        {injectOpen && (
          <InjectEventForm
            namespace={ns}
            onClose={() => setInjectOpen(false)}
            onInjected={(id) => setLastInjectedId(id)}
          />
        )}
      </Dialog>
    </PageContainer>
  )
}

/**
 * LiveTail subscribes to the events SSE stream and presents a 1000-event ring
 * buffer, newest first. Pausing buffers arrivals; resuming flushes them. New
 * rows flash briefly. A `dropped` frame from the server (client fell behind)
 * raises a warning banner.
 *
 * Three things here are deliberately bounded, because the naive version of
 * each was enough to block the main thread on a busy namespace and make the
 * whole console — sidebar included — feel unresponsive:
 *
 *   - **Rendered rows.** The buffer holds TAIL_CAP events; only WINDOW_SIZE of
 *     them are ever in the DOM. Committing all 1000 cost hundreds of
 *     milliseconds of layout per update.
 *   - **Updates.** Arrivals land in a ref and are applied on a FLUSH_MS timer,
 *     so a burst of 1000 events is a handful of renders rather than 1000.
 *   - **Timers.** Flash expiry is one sweeping interval over a Map of
 *     deadlines, not a setTimeout per row. Resuming from a full pause used to
 *     schedule up to 1000 timers at once.
 *
 * Looking back through history pauses the tail. The window is an offset into
 * the buffer, and the buffer shifts under it whenever an event arrives, so
 * scrollback and live append cannot both be true without the rows moving under
 * the operator mid-read. This is the same bargain a terminal makes.
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
  // Offset of the rendered window into `events`. 0 is the newest page, which
  // is the only position a live tail occupies.
  const [windowStart, setWindowStart] = useState(0)

  const pausedRef = useRef(paused)
  useEffect(() => {
    pausedRef.current = paused
  }, [paused])

  // Buffered while paused, applied on resume.
  const pendingRef = useRef<EventSummary[]>([])
  // Arrived since the last flush, applied on the flush timer.
  const arrivalsRef = useRef<EventSummary[]>([])
  const flushTimerRef = useRef<number | null>(null)
  // id → timestamp the flash expires. One sweep drains it.
  const flashExpiryRef = useRef<Map<number, number>>(new Map())
  const flashSweepRef = useRef<number | null>(null)

  /**
   * startFlashSweep runs at most one interval for the whole table. It prunes
   * expired ids and stops itself once nothing is flashing, so an idle tail
   * holds no timers.
   */
  const startFlashSweep = useCallback(() => {
    if (flashSweepRef.current != null) return
    flashSweepRef.current = window.setInterval(() => {
      const now = Date.now()
      let changed = false
      for (const [id, expiresAt] of flashExpiryRef.current) {
        if (expiresAt <= now) {
          flashExpiryRef.current.delete(id)
          changed = true
        }
      }
      if (changed) setFlashIds(new Set(flashExpiryRef.current.keys()))
      if (flashExpiryRef.current.size === 0 && flashSweepRef.current != null) {
        window.clearInterval(flashSweepRef.current)
        flashSweepRef.current = null
      }
    }, FLASH_SWEEP_MS)
  }, [])

  const flashAll = useCallback(
    (incoming: EventSummary[]) => {
      if (incoming.length === 0) return
      const expiresAt = Date.now() + FLASH_MS
      for (const e of incoming) flashExpiryRef.current.set(e.id, expiresAt)
      setFlashIds(new Set(flashExpiryRef.current.keys()))
      startFlashSweep()
    },
    [startFlashSweep],
  )

  /**
   * flush applies everything collected since the last tick as a single update.
   * The paused counter rides along so a paused tail receiving traffic is also
   * one render per tick rather than one per event.
   */
  const flush = useCallback(() => {
    flushTimerRef.current = null
    setPendingCount(pendingRef.current.length)

    const arrivals = arrivalsRef.current
    if (arrivals.length === 0) return
    arrivalsRef.current = []
    // Arrivals are collected oldest-first; the tail reads newest-first.
    const incoming = arrivals.reverse()
    setEvents((prev) => [...incoming, ...prev].slice(0, TAIL_CAP))
    flashAll(incoming)
  }, [flashAll])

  const scheduleFlush = useCallback(() => {
    if (flushTimerRef.current != null) return
    flushTimerRef.current = window.setTimeout(flush, FLUSH_MS)
  }, [flush])

  // The component is keyed by stream URL, so a filter change remounts it and
  // this cleanup is what stops the previous filter's timers. Route unmount
  // takes the same path.
  useEffect(
    () => () => {
      if (flushTimerRef.current != null) window.clearTimeout(flushTimerRef.current)
      if (flashSweepRef.current != null) window.clearInterval(flashSweepRef.current)
      flushTimerRef.current = null
      flashSweepRef.current = null
    },
    [],
  )

  const { connected } = useServerStream(streamUrl || null, {
    event: (data: unknown) => {
      const e = data as EventStreamMessage
      if (pausedRef.current) {
        pendingRef.current = [e, ...pendingRef.current].slice(0, TAIL_CAP)
      } else {
        arrivalsRef.current.push(e)
      }
      scheduleFlush()
    },
    dropped: (data: unknown) => {
      const d = data as { count?: number }
      setDroppedCount((c) => c + (d.count ?? 0))
    },
  })

  const maxWindowStart = Math.max(0, Math.floor((events.length - 1) / WINDOW_SIZE) * WINDOW_SIZE)
  // A live tail is pinned to the newest page; only a paused one can wander.
  const start = paused ? Math.min(windowStart, maxWindowStart) : 0
  const visible = events.slice(start, start + WINDOW_SIZE)
  const hasScrollback = events.length > WINDOW_SIZE

  const pause = () => {
    setPaused(true)
    pausedRef.current = true
  }

  const resume = () => {
    const buffered = pendingRef.current
    pendingRef.current = []
    setPendingCount(0)
    setPaused(false)
    pausedRef.current = false
    // Resuming returns to the newest page; the offsets below it no longer mean
    // what they meant before the buffered events were prepended.
    setWindowStart(0)
    if (buffered.length > 0) {
      setEvents((prev) => [...buffered, ...prev].slice(0, TAIL_CAP))
      flashAll(buffered)
    }
  }

  const showOlder = () => {
    // Scrollback and live append cannot coexist without rows moving under the
    // reader, so stepping back pauses.
    if (!paused) pause()
    setWindowStart((current) => Math.min(current + WINDOW_SIZE, maxWindowStart))
  }

  const showNewer = () => setWindowStart((current) => Math.max(current - WINDOW_SIZE, 0))

  return (
    <Stack gap={6}>
      <Stack gap={4} direction="horizontal" align="center" justify="between" wrap="wrap">
        <Token
          color={connected ? 'green' : 'gray'}
          label={connected ? 'Streaming' : 'Connecting / disconnected'}
        />
        <Button
          size="sm"
          variant="secondary"
          onClick={() => (paused ? resume() : pause())}
          label={paused ? `Resume${pendingCount > 0 ? ` (${pendingCount})` : ''}` : 'Pause'}
        />
      </Stack>

      {droppedCount > 0 && (
        <Banner
          status="warning"
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
          actions={<Button size="sm" onClick={onInject} label="Inject test event" />}
        />
      ) : (
        <>
          <TailTable
            namespace={namespace}
            items={visible}
            flashIds={flashIds}
            rowIndexStart={start + 1}
            totalRows={events.length}
          />
          {hasScrollback && (
            <Stack gap={4} direction="horizontal" align="center" justify="between" wrap="wrap">
              <span className="text-secondary text-sm" role="status" aria-live="polite">
                {`Showing ${start + 1}–${start + visible.length} of ${events.length} retained${
                  paused ? '' : ' — newest first, live'
                }`}
              </span>
              <Stack gap={2} direction="horizontal" align="center">
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={showNewer}
                  isDisabled={start === 0}
                  label="Newer"
                />
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={showOlder}
                  isDisabled={start >= maxWindowStart}
                  label={paused ? 'Older' : 'Older (pauses tail)'}
                />
              </Stack>
            </Stack>
          )}
        </>
      )}
    </Stack>
  )
}

/**
 * TailTable renders one window of the buffer.
 *
 * `items` is a slice, not the whole history, so the table carries the ARIA
 * bookkeeping that makes a windowed view legible to assistive tech:
 * aria-rowcount is the full retained count and each row's aria-rowindex is its
 * position in that count, not in the slice. Without them a screen reader
 * announces "row 3 of 60" for an event that is actually the 543rd retained.
 *
 * The header occupies row 1, so body rows start at rowIndexStart + 1.
 */
function TailTable({
  namespace,
  items,
  flashIds,
  rowIndexStart,
  totalRows,
}: {
  namespace: string
  items: EventSummary[]
  flashIds: Set<number>
  rowIndexStart: number
  totalRows: number
}) {
  return (
    <Stack className="min-w-0">
      <Table
        aria-label="Live events"
        aria-rowcount={totalRows + 1}
        columns={['Occurred', 'Subject', 'Object', 'Action', 'Weight'].map((key) => ({
          key,
          header: key,
          width: proportional(1),
        }))}
      >
        <TableHeader>
          <TableRow aria-rowindex={1}>
            <TableHeaderCell>Occurred</TableHeaderCell>
            <TableHeaderCell>Subject</TableHeaderCell>
            <TableHeaderCell>Object</TableHeaderCell>
            <TableHeaderCell>Action</TableHeaderCell>
            <TableHeaderCell className="text-right">Weight</TableHeaderCell>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((e, i) => (
            <TableRow
              key={e.id}
              aria-rowindex={rowIndexStart + i + 1}
              className={
                flashIds.has(e.id)
                  ? 'bg-accent-muted motion-safe:transition-colors'
                  : 'motion-safe:transition-colors'
              }
            >
              <TableCell className="text-secondary text-xs tabular-nums">
                {new Date(e.occurred_at).toLocaleTimeString()}
              </TableCell>
              <TableCell>
                <Link
                  to={`/ns/${encodeURIComponent(namespace)}/subjects/${encodeURIComponent(e.subject_id)}`}
                  className="text-primary text-sm font-medium"
                >
                  <code>{e.subject_id}</code>
                </Link>
              </TableCell>
              <TableCell>
                <code className="text-secondary text-xs">{e.object_id}</code>
              </TableCell>
              <TableCell>
                <Token color="gray" label={e.action} />
              </TableCell>
              <TableCell className="text-right tabular-nums">{e.weight.toFixed(2)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Stack>
  )
}

function SummarySidebar({ namespace }: { namespace: string }) {
  const [window, setWindow] = useState<EventsSummaryWindow>('1m')
  const summary = useEventsSummary(namespace, window)

  return (
    <Stack gap={6}>
      <SegmentedControl
        label="Event summary time window"
        value={window}
        onChange={(next) => setWindow(next as EventsSummaryWindow)}
      >
        {WINDOWS.map((w) => (
          <SegmentedControlItem key={w} value={w} label={w} />
        ))}
      </SegmentedControl>
      <QueryFeedback query={summary} label="Event summary" />
      {summary.isLoading && <Skeleton className="h-24 w-full" />}
      {summary.data && (
        <>
          <Stack align="center" gap={4} direction="horizontal" wrap="wrap">
            <SummaryTile label={`Events (${window})`} value={summary.data.total.toLocaleString()} />
            <SummaryTile label="Rate / s" value={summary.data.rate_per_second.toFixed(2)} />
          </Stack>

          <ActionMix data={summary.data} />

          <Stack gap={6}>
            <span className="text-secondary text-xs uppercase tracking-wide">Over time</span>
            {summary.data && summary.data.series.length > 0 ? (
              <TimeSeriesChart
                data={summary.data.series.map((b) => ({ ts: b.ts, count: b.count }))}
                series={[{ key: 'count', label: 'Events', color: 'var(--color-success)' }]}
                height={140}
              />
            ) : (
              <p className="text-secondary text-sm">No events in this window.</p>
            )}
          </Stack>
        </>
      )}
    </Stack>
  )
}

function SummaryTile({ label, value }: { label: string; value: string }) {
  return (
    <Card className="flex-1 min-w-30">
      <Stack gap={6}>
        <span className="text-secondary text-xs uppercase tracking-wide">{label}</span>
        <span className="text-primary text-xl font-semibold tabular-nums">{value}</span>
      </Stack>
    </Card>
  )
}

function ActionMix({ data }: { data: EventsSummaryResponse | undefined }) {
  if (!data || data.by_action.length === 0) {
    return null
  }
  const total = data.total || 1
  return (
    <Stack gap={6}>
      <span className="text-secondary text-xs uppercase tracking-wide">Action mix</span>
      <Stack gap={6}>
        {data.by_action.map((a) => {
          const pct = Math.round((a.count / total) * 100)
          return (
            <Stack gap={6} key={a.action}>
              <Stack gap={4} direction="horizontal" align="center" justify="between">
                <span className="text-primary text-sm">{a.action}</span>
                <span className="text-secondary text-xs tabular-nums">
                  {a.count.toLocaleString()} ({pct}%)
                </span>
              </Stack>
              <ProgressBar value={pct} label={`${a.action}: ${pct}%`} />
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
      <Layout
        header={
          <DialogHeader
            title={`Inject test event into ${namespace}`}
            subtitle="Proxied through the admin event injection endpoint. Lands in the same events table the ingest worker writes to, and flashes in the live tail as it arrives."
            onOpenChange={onClose}
          />
        }
        content={
          <LayoutContent>
            <Stack gap={6}>
              {inject.error && (
                <Banner status="error" title="Inject failed" description={inject.error.message} />
              )}
              <TextInput
                label="Subject ID"
                value={subjectId}
                onChange={setSubjectId}
                placeholder="user-42"
              />
              <TextInput
                label="Object ID"
                value={objectId}
                onChange={setObjectId}
                placeholder="item-100"
              />
              <TextInput
                label="Action"
                description="Matches an entry in the namespace action_weights map."
                value={action}
                onChange={setAction}
              />
            </Stack>
          </LayoutContent>
        }
        footer={
          <LayoutFooter>
            <Stack direction="horizontal" gap={2} align="center" hAlign="end">
              <Button type="button" variant="ghost" onClick={onClose} label="Cancel" />
              <Button
                type="submit"
                isDisabled={!canSubmit}
                label={inject.isPending ? 'Injecting…' : 'Inject'}
              />
            </Stack>
          </LayoutFooter>
        }
      />
    </form>
  )
}
