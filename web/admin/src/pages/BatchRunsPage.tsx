import { useState } from 'react'
import { BATCH_PAGE_SIZE, useBatchRuns } from '../hooks/useBatchRuns'
import { useTriggerBatch } from '../hooks/useTriggerBatch'
import ErrorBanner from '../components/ErrorBanner'
import {
  Badge,
  Button,
  CodeBadge,
  EmptyState,
  LoadingState,
  PageHeader,
  PageShell,
} from '../components/ui'
import type { BatchRunLog } from '../types'
import { useActiveNamespace } from '../context/useActiveNamespace'
import BatchRunModal from './batch-runs/BatchRunModal'
import BatchRunsTable from './batch-runs/BatchRunsTable'
import FilterChips, { type StatusFilter } from './batch-runs/FilterChips'
import SummaryStrip from './batch-runs/SummaryStrip'

export default function BatchRunsPage() {
  const { namespace } = useActiveNamespace()

  return <BatchRunsContent key={namespace ?? 'none'} namespace={namespace} />
}

function BatchRunsContent({ namespace }: { namespace: string | null }) {
  const [selectedRun, setSelectedRun] = useState<BatchRunLog | null>(null)
  const [page, setPage] = useState(0)
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')

  const { data, error, isLoading } = useBatchRuns(
    namespace || undefined,
    page,
    statusFilter === 'all' ? '' : statusFilter,
  )
  const runNow = useTriggerBatch(namespace ?? '')

  const runs = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / BATCH_PAGE_SIZE)
  const hasRunning = (data?.stats.running ?? 0) > 0

  const counts: Record<StatusFilter, number> = {
    all: data?.stats.total ?? 0,
    running: data?.stats.running ?? 0,
    ok: data?.stats.ok ?? 0,
    failed: data?.stats.failed ?? 0,
  }

  function handleFilterChange(nextFilter: StatusFilter) {
    setStatusFilter(nextFilter)
    setPage(0)
  }

  return (
    <PageShell>
      <PageHeader
        title="Batch Runs"
        actions={
          <div className="flex items-center gap-3">
            {hasRunning && (
              <Badge tone="accent" dot>
                Live
              </Badge>
            )}
            {namespace && (
              <Button
                size="sm"
                onClick={() => runNow.mutate()}
                disabled={runNow.isPending}
              >
                {runNow.isPending ? 'Running...' : 'Run now'}
              </Button>
            )}
          </div>
        }
      />

      {error && <ErrorBanner message="Failed to load batch runs." />}
      {isLoading && <LoadingState />}

      {data && runs.length === 0 && total === 0 && (
        <EmptyState>
          No runs yet - run <CodeBadge>make run-cron</CodeBadge> to populate batch history.
        </EmptyState>
      )}

      {data && total > 0 && (
        <>
          <SummaryStrip runs={runs} total={total} />

          <FilterChips
            value={statusFilter}
            onChange={handleFilterChange}
            counts={counts}
          />

          <BatchRunsTable
            runs={runs}
            statusFilter={statusFilter}
            page={page}
            total={total}
            totalPages={totalPages}
            onSelectRun={setSelectedRun}
            onPreviousPage={() => setPage(p => p - 1)}
            onNextPage={() => setPage(p => p + 1)}
          />
        </>
      )}

      {selectedRun && (
        <BatchRunModal run={selectedRun} onClose={() => setSelectedRun(null)} />
      )}
    </PageShell>
  )
}
