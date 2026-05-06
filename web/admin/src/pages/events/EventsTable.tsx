import { Button, CodeBadge, Panel, Table, Tbody, Td, Th, Thead, Tr } from '../../components/ui'
import type { EventSummary } from '../../types'
import { formatCount, formatDateTime } from '../../utils/format'

export default function EventsTable({
  events,
  offset,
  pageSize,
  pageEnd,
  total,
  subjectFilter,
  onPreviousPage,
  onNextPage,
}: {
  events: EventSummary[]
  offset: number
  pageSize: number
  pageEnd: number
  total: number
  subjectFilter: string
  onPreviousPage: () => void
  onNextPage: () => void
}) {
  const totalPages = Math.ceil(total / pageSize)

  return (
    <Panel bodyClassName="overflow-x-auto">
      <Table>
        <Thead>
          <Th>ID</Th>
          <Th>Time</Th>
          <Th>Subject ID</Th>
          <Th>Object ID</Th>
          <Th>Action</Th>
          <Th align="right">Weight</Th>
        </Thead>
        <Tbody>
          {events.length === 0 && (
            <Tr>
              <Td colSpan={6} muted className="text-center py-6 italic">
                No events{subjectFilter ? ' for this subject' : ' on this page'}
              </Td>
            </Tr>
          )}
          {events.map(ev => (
            <Tr key={ev.id} hoverable>
              <Td mono>{ev.id}</Td>
              <Td muted mono className="whitespace-nowrap">{formatDateTime(ev.occurred_at)}</Td>
              <Td><CodeBadge>{ev.subject_id}</CodeBadge></Td>
              <Td><CodeBadge>{ev.object_id}</CodeBadge></Td>
              <Td className="font-medium">{ev.action}</Td>
              <Td align="right" mono>{ev.weight.toFixed(2)}</Td>
            </Tr>
          ))}
        </Tbody>
      </Table>

      {totalPages > 1 && (
        <div className="flex items-center justify-between border-t border-default px-2 pt-3">
          <span className="text-xs text-muted">
            {offset + 1}-{pageEnd} of {formatCount(total)}
          </span>
          <div className="flex gap-1">
            <Button
              size="sm"
              variant="ghost"
              disabled={offset === 0}
              onClick={onPreviousPage}
            >
              Prev
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={pageEnd >= total}
              onClick={onNextPage}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </Panel>
  )
}
