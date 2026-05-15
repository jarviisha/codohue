import { useParams } from 'react-router-dom'
import {
  Button,
  EmptyState,
  Field,
  LoadingState,
  Notice,
  Pagination,
  Panel,
  Select,
  StatusToken,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Toolbar,
  Tr,
  useRegisterCommand,
} from '@/components/ui'
import { useBatchRunsList } from '@/services/batchRuns'
import type { BatchRunStatusFilter } from '@/services/batchRuns'
import { formatNumber, formatRelative } from '@/utils/format'
import { formatPhaseDuration, runStatusLabel, runStatusToken } from './helpers'
import { useRunsFilter } from './useRunsFilter'

const STATUSES: { value: BatchRunStatusFilter; label: string }[] = [
  { value: '', label: 'all' },
  { value: 'running', label: 'running' },
  { value: 'ok', label: 'ok' },
  { value: 'failed', label: 'failed' },
]

// Catalog re-embed orchestration runs. Rows have no phase breakdown — the
// useful columns are the target strategy and the failure message.
//
// While a run is open the error_message column carries the encoded reembed
// target ("reembed:<id>/<version>"). The backend's summary endpoint strips
// that encoding for the Status tab, but the raw list of runs goes through
// the generic batch_run_logs reader so we still see it here. Display the
// raw value but only when the run is closed AND failed.
export default function ReEmbedsPage() {
  const { name = '' } = useParams<{ name: string }>()
  const { filter, setFilter } = useRunsFilter()
  const runs = useBatchRunsList({
    namespace: name,
    kind: 'reembed',
    status: filter.status || undefined,
    limit: filter.limit,
    offset: filter.offset,
  })

  useRegisterCommand(
    `ns.${name}.batchRuns.reembed.refresh`,
    `Refresh ${name} catalog re-embed runs`,
    () => void runs.refetch(),
    name,
  )

  const rows = runs.data?.items ?? []

  return (
    <Panel
      title="catalog re-embed runs"
      actions={
        <Button
          variant="ghost"
          size="sm"
          loading={runs.isFetching && !runs.isLoading}
          onClick={() => void runs.refetch()}
        >
          Refresh
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        {runs.isError ? (
          <Notice tone="fail" title="Failed to load re-embed runs">
            {(runs.error as Error)?.message ?? 'Unable to load batch runs.'}
          </Notice>
        ) : null}

        <Toolbar>
          <Field label="status" htmlFor="reembed-status">
            <Select
              id="reembed-status"
              selectSize="sm"
              value={filter.status}
              onChange={(event) =>
                setFilter({ status: event.target.value as BatchRunStatusFilter })
              }
            >
              {STATUSES.map((s) => (
                <option key={s.label} value={s.value}>{s.label}</option>
              ))}
            </Select>
          </Field>
          <Field label="limit" htmlFor="reembed-limit">
            <Select
              id="reembed-limit"
              selectSize="sm"
              value={String(filter.limit)}
              onChange={(event) => setFilter({ limit: Number(event.target.value) })}
            >
              {[10, 20, 50, 100].map((v) => (
                <option key={v} value={v}>{v}</option>
              ))}
            </Select>
          </Field>
        </Toolbar>

        {runs.isLoading ? (
          <LoadingState rows={5} label="loading re-embed runs" />
        ) : rows.length === 0 && !runs.isError ? (
          <EmptyState
            title="No re-embed runs match"
            description="Trigger one from the catalog Config tab; runs appear here as they start."
          />
        ) : (
          <>
            <Table>
              <Thead>
                <Tr>
                  <Th>status</Th>
                  <Th>id</Th>
                  <Th>started</Th>
                  <Th>completed</Th>
                  <Th align="right">duration</Th>
                  <Th align="right">processed</Th>
                  <Th>error</Th>
                </Tr>
              </Thead>
              <Tbody>
                {rows.map((row) => {
                  const closedFailure =
                    row.completed_at && !row.success && row.error_message
                      ? row.error_message
                      : ''
                  return (
                    <Tr key={row.id}>
                      <Td>
                        <StatusToken
                          state={runStatusToken(row)}
                          label={runStatusLabel(row)}
                        />
                      </Td>
                      <Td mono>#{row.id}</Td>
                      <Td mono>{formatRelative(row.started_at)}</Td>
                      <Td mono>
                        {row.completed_at ? formatRelative(row.completed_at) : '—'}
                      </Td>
                      <Td mono align="right">{formatPhaseDuration(row.duration_ms)}</Td>
                      <Td mono align="right">{formatNumber(row.entities_processed)}</Td>
                      <Td className="max-w-xl truncate" title={closedFailure}>
                        {closedFailure || '—'}
                      </Td>
                    </Tr>
                  )
                })}
              </Tbody>
            </Table>

            <Pagination
              offset={filter.offset}
              limit={filter.limit}
              total={runs.data?.total}
              onOffsetChange={(next) => setFilter({ offset: next })}
            />
          </>
        )}
      </div>
    </Panel>
  )
}
