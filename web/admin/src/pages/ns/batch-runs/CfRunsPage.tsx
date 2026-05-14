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
import {
  useBatchRunsList,
  useTriggerBatchRun,
} from '@/services/batchRuns'
import type { BatchRunStatusFilter } from '@/services/batchRuns'
import { formatNumber, formatRelative } from '@/utils/format'
import {
  formatPhaseDuration,
  phaseToken,
  runStatusLabel,
  runStatusToken,
} from './helpers'
import { useRunsFilter } from './useRunsFilter'

const STATUSES: { value: BatchRunStatusFilter; label: string }[] = [
  { value: '', label: 'all' },
  { value: 'running', label: 'running' },
  { value: 'ok', label: 'ok' },
  { value: 'failed', label: 'failed' },
]

// CF runs tab — sparse CF, dense item embeddings, trending. Rows have full
// per-phase breakdown columns.
export default function CfRunsPage() {
  const { name = '' } = useParams<{ name: string }>()
  const { filter, setFilter } = useRunsFilter()
  const runs = useBatchRunsList({
    namespace: name,
    kind: 'cf',
    status: filter.status || undefined,
    limit: filter.limit,
    offset: filter.offset,
  })
  const triggerBatch = useTriggerBatchRun()

  useRegisterCommand(
    `ns.${name}.batchRuns.cf.refresh`,
    `Refresh ${name} CF runs`,
    () => void runs.refetch(),
    name,
  )

  const rows = runs.data?.items ?? []

  return (
    <Panel
      title="cf runs"
      actions={
        <>
          <Button
            variant="ghost"
            size="sm"
            loading={runs.isFetching && !runs.isLoading}
            onClick={() => void runs.refetch()}
          >
            Refresh
          </Button>
          <Button
            variant="primary"
            size="sm"
            loading={triggerBatch.isPending}
            onClick={() => triggerBatch.mutate(name)}
          >
            Run batch now
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {triggerBatch.isError ? (
          <Notice tone="fail" title="Trigger failed">
            {(triggerBatch.error as Error)?.message ?? 'Unable to start a batch run.'}
          </Notice>
        ) : null}
        {triggerBatch.isSuccess && triggerBatch.data ? (
          <Notice
            tone="ok"
            title={`Batch run #${triggerBatch.data.id} queued`}
            onDismiss={() => triggerBatch.reset()}
          >
            The list refreshes as the run lands.
          </Notice>
        ) : null}

        {runs.isError ? (
          <Notice tone="fail" title="Failed to load CF runs">
            {(runs.error as Error)?.message ?? 'Unable to load batch runs.'}
          </Notice>
        ) : null}

        <Toolbar>
          <Field label="status" htmlFor="cf-status">
            <Select
              id="cf-status"
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
          <Field label="limit" htmlFor="cf-limit">
            <Select
              id="cf-limit"
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
          <LoadingState rows={5} label="loading cf runs" />
        ) : rows.length === 0 && !runs.isError ? (
          <EmptyState
            title="No CF runs match"
            description="Adjust the status filter, or trigger a run from the panel above."
          />
        ) : (
          <>
            <Table>
              <Thead>
                <Tr>
                  <Th>status</Th>
                  <Th>id</Th>
                  <Th>trigger</Th>
                  <Th>started</Th>
                  <Th align="right">duration</Th>
                  <Th align="right">subjects</Th>
                  <Th>phase 1</Th>
                  <Th>phase 2</Th>
                  <Th>phase 3</Th>
                </Tr>
              </Thead>
              <Tbody>
                {rows.map((row) => (
                  <Tr key={row.id}>
                    <Td>
                      <StatusToken
                        state={runStatusToken(row)}
                        label={runStatusLabel(row)}
                        title={row.error_message ?? undefined}
                      />
                    </Td>
                    <Td mono>#{row.id}</Td>
                    <Td mono>{row.trigger_source}</Td>
                    <Td mono>{formatRelative(row.started_at)}</Td>
                    <Td mono align="right">{formatPhaseDuration(row.duration_ms)}</Td>
                    <Td mono align="right">{formatNumber(row.subjects_processed)}</Td>
                    <Td>
                      <StatusToken
                        state={phaseToken(row.phase1_ok)}
                        label={formatPhaseDuration(row.phase1_duration_ms)}
                        title={row.phase1_error ?? undefined}
                      />
                    </Td>
                    <Td>
                      <StatusToken
                        state={phaseToken(row.phase2_ok)}
                        label={formatPhaseDuration(row.phase2_duration_ms)}
                        title={row.phase2_error ?? undefined}
                      />
                    </Td>
                    <Td>
                      <StatusToken
                        state={phaseToken(row.phase3_ok)}
                        label={formatPhaseDuration(row.phase3_duration_ms)}
                        title={row.phase3_error ?? undefined}
                      />
                    </Td>
                  </Tr>
                ))}
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
