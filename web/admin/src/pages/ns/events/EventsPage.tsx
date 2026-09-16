import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import {
  Badge,
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
import PageHeader from '@/components/shell/PageHeader'
import TimeSeriesChart from '@/components/charts/TimeSeriesChart'

const TAIL_CAP = 1000
const FLASH_MS = 1500
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
    <div className="px-6 py-6">
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full" wrap="wrap">
          <Stack gap={1}>
            <h1 className="text-primary text-xl font-semibold">Events</h1>
            <p className="text-secondary text-sm">Live ingest tail, forward-only</p>
          </Stack>
          <Button size="sm" onClick={() => setInjectOpen(true)} label="Inject test event" />
        </Stack>
      </PageHeader>

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_20rem] gap-6">
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
            {action && (
              <Badge variant="info" label={`action = ${action}`} />
            )}
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
      </div>

      <Dialog isOpen={injectOpen} onOpenChange={setInjectOpen} width={560} purpose="form">
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
    <Stack gap={6}>
      <Stack gap={4} direction="horizontal" align="center" justify="between" wrap="wrap">
        <Badge variant={connected ? 'success' : 'neutral'} label={connected ? 'streaming' : 'offline'} />
        <Button
          size="sm"
          variant="secondary"
          
          onClick={() => (paused ? resume() : setPaused(true))}
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
          actions={
            <Button size="sm" onClick={onInject} label="Inject test event" />
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
      <Table>
        <TableHeader>
          <TableRow>
            <TableHeaderCell>Occurred</TableHeaderCell>
            <TableHeaderCell>Subject</TableHeaderCell>
            <TableHeaderCell>Object</TableHeaderCell>
            <TableHeaderCell>Action</TableHeaderCell>
            <TableHeaderCell className="text-right" >Weight</TableHeaderCell>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((e) => (
            <TableRow
              key={e.id}
              className={
                flashIds.has(e.id) ? 'bg-accent-muted transition-colors' : 'transition-colors'
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
                <Badge variant="neutral" label={e.action} />
              </TableCell>
              <TableCell  className="text-right tabular-nums">
                {e.weight.toFixed(2)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
  )
}

function SummarySidebar({ namespace }: { namespace: string }) {
  const [window, setWindow] = useState<EventsSummaryWindow>('1m')
  const summary = useEventsSummary(namespace, window)

  return (
    <Stack gap={6}>
      <Stack gap={4} direction="horizontal" align="center">
        {WINDOWS.map((w) => (
          <Button
            key={w}
            size="sm"
            variant="primary"
            
            onClick={() => setWindow(w)} label={w} />
        ))}
      </Stack>

      <Stack align="center" gap={4} direction="horizontal" wrap="wrap">
        <SummaryTile
          label={`Events (${window})`}
          value={(summary.data?.total ?? 0).toLocaleString()}
        />
        <SummaryTile label="Rate / s" value={(summary.data?.rate_per_second ?? 0).toFixed(2)} />
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
              <div className="h-1.5 w-full rounded-full bg-muted">
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
