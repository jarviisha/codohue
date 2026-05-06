import { CodeBadge, Panel, Table, Tbody, Td, Th, Thead, Tr } from '../../components/ui'
import type { EventSummary } from '../../types'
import { formatDateTime } from '../../utils/format'

export default function EventsTable({ events }: { events: EventSummary[] }) {
  return (
    <Panel bodyClassName="overflow-x-auto">
      <Table>
        <Thead>
          <Th>Time</Th>
          <Th>Subject ID</Th>
          <Th>Object ID</Th>
          <Th>Action</Th>
          <Th align="right">Weight</Th>
        </Thead>
        <Tbody>
          {events.map(ev => (
            <Tr key={ev.id} hoverable>
              <Td muted mono className="whitespace-nowrap">{formatDateTime(ev.occurred_at)}</Td>
              <Td><CodeBadge>{ev.subject_id}</CodeBadge></Td>
              <Td><CodeBadge>{ev.object_id}</CodeBadge></Td>
              <Td className="font-medium">{ev.action}</Td>
              <Td align="right" mono>{ev.weight.toFixed(2)}</Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </Panel>
  )
}
