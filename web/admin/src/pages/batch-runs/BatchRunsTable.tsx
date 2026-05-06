import { BATCH_PAGE_SIZE } from '../../hooks/useBatchRuns'
import { Button, CodeBadge, Panel, Table, Tbody, Td, Th, Thead, Tr } from '../../components/ui'
import type { BatchRunLog } from '../../types'
import { fmtDateShort } from './format'
import type { StatusFilter } from './FilterChips'
import { RunStatus, TriggerBadge } from './badges'

export default function BatchRunsTable({
  runs,
  statusFilter,
  page,
  total,
  totalPages,
  onSelectRun,
  onPreviousPage,
  onNextPage,
}: {
  runs: BatchRunLog[]
  statusFilter: StatusFilter
  page: number
  total: number
  totalPages: number
  onSelectRun: (run: BatchRunLog) => void
  onPreviousPage: () => void
  onNextPage: () => void
}) {
  return (
    <Panel bodyClassName="overflow-x-auto">
      <Table>
        <Thead>
          <Th>ID</Th>
          <Th>Namespace</Th>
          <Th>Trigger</Th>
          <Th>Started</Th>
          <Th>Completed</Th>
          <Th>Duration</Th>
          <Th>Subjects</Th>
          <Th>Status</Th>
        </Thead>
        <Tbody>
          {runs.length === 0 && (
            <Tr>
              <Td colSpan={8} muted className="text-center py-6 italic">
                No {statusFilter === 'all' ? '' : statusFilter + ' '}runs on this page
              </Td>
            </Tr>
          )}
          {runs.map(run => (
            <Tr
              key={run.id}
              hoverable
              onClick={() => onSelectRun(run)}
              className="cursor-pointer"
            >
              <Td mono>{run.id}</Td>
              <Td>
                <CodeBadge>{run.namespace}</CodeBadge>
              </Td>
              <Td>
                <TriggerBadge source={run.trigger_source} />
              </Td>
              <Td mono>{fmtDateShort(run.started_at)}</Td>
              <Td mono>
                {run.completed_at
                  ? fmtDateShort(run.completed_at)
                  : <em className="not-italic text-accent">in progress</em>}
              </Td>
              <Td mono>
                {run.duration_ms != null
                  ? `${run.duration_ms} ms`
                  : run.completed_at ? '–' : '—'}
              </Td>
              <Td mono>{run.subjects_processed}</Td>
              <Td>
                <RunStatus run={run} />
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>

      {totalPages > 1 && (
        <div className="flex items-center justify-between border-t border-default px-2 pt-3">
          <span className="text-xs text-muted">
            {page * BATCH_PAGE_SIZE + 1}–{Math.min((page + 1) * BATCH_PAGE_SIZE, total)} of {total}
          </span>
          <div className="flex gap-1">
            <Button
              size="sm"
              variant="ghost"
              disabled={page === 0}
              onClick={onPreviousPage}
            >
              Prev
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={page >= totalPages - 1}
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
