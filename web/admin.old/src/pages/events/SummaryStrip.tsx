import { MetricTile } from '../../components/ui'
import type { EventSummary } from '../../types'
import { formatCount, formatDateTime } from '../../utils/format'

export default function SummaryStrip({
  events,
  total,
  subjectFilter,
}: {
  events: EventSummary[]
  total: number
  subjectFilter: string
}) {
  const subjects = new Set(events.map(event => event.subject_id))
  const actionCounts = events.reduce<Record<string, number>>((counts, event) => {
    counts[event.action] = (counts[event.action] ?? 0) + 1
    return counts
  }, {})
  const topAction = Object.entries(actionCounts).sort((a, b) => b[1] - a[1])[0]
  const latestEvent = events[0]

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <MetricTile
        label="Total events"
        value={formatCount(total)}
        sub={subjectFilter ? `filtered by ${subjectFilter}` : undefined}
        valueClassName="text-lg"
      />
      <MetricTile
        label="Latest event"
        value={latestEvent ? formatDateTime(latestEvent.occurred_at) : '-'}
        sub={latestEvent ? `${latestEvent.action} by ${latestEvent.subject_id}` : undefined}
        valueClassName="text-sm"
      />
      <MetricTile
        label="Top action"
        value={topAction ? topAction[0] : '-'}
        sub={topAction ? `${formatCount(topAction[1])} visible events` : undefined}
        valueClassName="text-lg"
      />
      <MetricTile
        label="Visible subjects"
        value={formatCount(subjects.size)}
        sub={`${formatCount(events.length)} visible events`}
        valueClassName="text-lg"
      />
    </div>
  )
}
