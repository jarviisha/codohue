import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import {
  Button,
  EmptyState,
  Field,
  Input,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  Pagination,
  Panel,
  Select,
  StatusToken,
  Switch,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Toolbar,
  Tr,
  useRegisterCommand,
} from '@/components/ui'
import { useEvents, type EventSummary } from '@/services/events'
import { formatNumber, formatRelative, formatTimestamp } from '@/utils/format'
import InjectEventModal from './InjectEventModal'
import { useEventsFilter } from './useEventsFilter'

// Recent events for a namespace + inject test events. The live-tail toggle
// flips between idle (no poll) and a 2s refetch cadence so operators can
// watch ingest in near real time without grinding the backend.
const LIVE_TAIL_INTERVAL_MS = 2_000

export default function EventsListPage() {
  const { name = '' } = useParams<{ name: string }>()
  const { filter, setFilter } = useEventsFilter()
  const [live, setLive] = useState(false)
  const [showInject, setShowInject] = useState(false)

  const events = useEvents({
    namespace: name,
    limit: filter.limit,
    offset: filter.offset,
    subject_id: filter.subjectID || undefined,
    refetchIntervalMs: live ? LIVE_TAIL_INTERVAL_MS : 0,
  })

  // Tick every second so the age (Δ) column re-renders while the page is
  // open. Cheap — just bumps a counter that React picks up.
  const [, setTick] = useState(0)
  useEffect(() => {
    const id = window.setInterval(() => setTick((t) => t + 1), 1_000)
    return () => window.clearInterval(id)
  }, [])

  useRegisterCommand(
    `ns.${name}.events.refresh`,
    `Refresh ${name} events`,
    () => void events.refetch(),
    name,
  )
  useRegisterCommand(
    `ns.${name}.events.inject`,
    `Inject event in ${name}`,
    () => setShowInject(true),
    name,
  )
  useRegisterCommand(
    `ns.${name}.events.liveTail`,
    `Toggle ${name} events live tail`,
    () => setLive((v) => !v),
    name,
  )

  const rows = events.data?.items ?? []
  const lastEvent: EventSummary | undefined = rows[0]

  return (
    <PageShell>
      <PageHeader
        title="events"
        actions={
          <Button variant="primary" onClick={() => setShowInject(true)}>
            Inject event
          </Button>
        }
      />

      <Panel title="recent events">
        <div className="flex flex-col gap-4">
          {events.isError ? (
            <Notice tone="fail" title="Failed to load events">
              {(events.error as Error)?.message ?? 'Unable to load events.'}
            </Notice>
          ) : null}

          <Toolbar>
            <Field label="subject_id" htmlFor="events-subject-id">
              <Input
                id="events-subject-id"
                inputSize="sm"
                value={filter.subjectID}
                placeholder="exact match"
                onChange={(event) => setFilter({ subject_id: event.target.value })}
              />
            </Field>
            <Field label="limit" htmlFor="events-limit">
              <Select
                id="events-limit"
                selectSize="sm"
                value={String(filter.limit)}
                onChange={(event) => setFilter({ limit: Number(event.target.value) })}
              >
                {[25, 50, 100, 200].map((value) => (
                  <option key={value} value={value}>{value}</option>
                ))}
              </Select>
            </Field>
            <div className="ml-auto flex items-center gap-3">
              <Button
                variant="ghost"
                size="sm"
                loading={events.isFetching && !events.isLoading}
                onClick={() => void events.refetch()}
              >
                Refresh
              </Button>
              <div className="flex items-center gap-2">
                <span className="font-mono text-xs uppercase tracking-[0.04em] text-secondary">
                  live tail
                </span>
                <Switch
                  checked={live}
                  onChange={setLive}
                  ariaLabel="Toggle events live tail"
                />
              </div>
            </div>
          </Toolbar>

          {events.isLoading ? (
            <LoadingState rows={8} label="loading events" />
          ) : rows.length === 0 && !events.isError ? (
            <EmptyState
              title="No events match"
              description="Adjust the subject_id filter or ingest more events to populate the list."
            />
          ) : (
            <>
              <Table>
                <Thead>
                  <Tr>
                    <Th>time</Th>
                    <Th>action</Th>
                    <Th>subject</Th>
                    <Th>object</Th>
                    <Th align="right">weight</Th>
                    <Th align="right">Δ</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {rows.map((event) => (
                    <Tr key={event.id}>
                      <Td mono>{formatTimestamp(event.occurred_at)}</Td>
                      <Td mono>{event.action}</Td>
                      <Td mono>{event.subject_id}</Td>
                      <Td mono>{event.object_id}</Td>
                      <Td mono align="right">{formatNumber(event.weight)}</Td>
                      <Td mono align="right">{formatRelative(event.occurred_at)}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>

              <Pagination
                offset={filter.offset}
                limit={filter.limit}
                total={events.data?.total}
                onOffsetChange={(next) => setFilter({ offset: next })}
              />
            </>
          )}

          <LiveTailStrip
            live={live}
            lastEvent={lastEvent}
            onToggle={() => setLive((v) => !v)}
          />
        </div>
      </Panel>

      <InjectEventModal
        namespace={name}
        open={showInject}
        onClose={() => setShowInject(false)}
      />
    </PageShell>
  )
}

interface LiveTailStripProps {
  live: boolean
  lastEvent: EventSummary | undefined
  onToggle: () => void
}

// Bottom strip mirroring the BUILD_PLAN §7.3 mockup: [RUN]/[IDLE] token,
// latest event timestamp, and a pause/resume affordance.
function LiveTailStrip({ live, lastEvent, onToggle }: LiveTailStripProps) {
  return (
    <div className="flex items-center justify-between border-t border-default pt-3 text-sm">
      <div className="flex items-center gap-3">
        <StatusToken state={live ? 'run' : 'idle'} />
        <span className="font-mono text-xs text-secondary">
          {live ? 'streaming' : 'paused'}
        </span>
        {lastEvent ? (
          <span className="font-mono text-xs text-muted">
            · last @ {formatTimestamp(lastEvent.occurred_at)}
          </span>
        ) : null}
      </div>
      <Button variant="ghost" size="sm" onClick={onToggle}>
        {live ? 'pause' : 'resume'}
      </Button>
    </div>
  )
}
