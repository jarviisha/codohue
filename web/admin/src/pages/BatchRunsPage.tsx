import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useBatchRuns, BATCH_PAGE_SIZE } from '../hooks/useBatchRuns'
import { useTriggerBatch } from '../hooks/useTriggerBatch'
import ErrorBanner from '../components/ErrorBanner'
import {
  Button,
  CodeBadge,
  EmptyState,
  Modal,
  PageHeader,
  Panel,
  Table,
  Thead,
  Th,
  Tbody,
  Tr,
  Td,
} from '../components/ui'
import type { BatchRunLog, LogEntry } from '../types'
import { useActiveNamespace } from '../context/NamespaceContext'

// ─── helpers ─────────────────────────────────────────────────────────────────

function fmtDate(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
  const offset = -d.getTimezoneOffset()
  const tzLabel = offset === 0 ? 'UTC' : `UTC${offset > 0 ? '+' : ''}${offset / 60}`
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())} (${tz || tzLabel})`
}

function fmtDateShort(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

// ─── sub-components ───────────────────────────────────────────────────────────

interface PhaseRowProps {
  label: string
  ok: boolean | null | undefined
  durMs: number | null | undefined
  counts: { label: string; value: number | null | undefined }[]
  error: string | null | undefined
  skipped?: boolean
}

function PhaseRow({ label, ok, durMs, counts, error, skipped }: PhaseRowProps) {
  const cell = 'py-1.5 text-[11px]'
  if (skipped) {
    return (
      <Tr>
        <Td className={`${cell} w-28`}>{label}</Td>
        <Td colSpan={3} muted className={`${cell} italic`}>skipped</Td>
      </Tr>
    )
  }
  if (ok == null) {
    return (
      <Tr>
        <Td className={`${cell} w-28`}>{label}</Td>
        <Td colSpan={3} muted className={`${cell} italic`}>no data</Td>
      </Tr>
    )
  }
  return (
    <Tr>
      <Td className={`${cell} w-28`}>{label}</Td>
      <Td className={cell}>
        {ok
          ? <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-medium bg-success-bg text-success">✓ OK</span>
          : <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-medium bg-danger-bg text-danger">✗ Failed</span>}
      </Td>
      <Td mono muted className={cell}>
        {durMs != null ? `${durMs} ms` : '—'}
      </Td>
      <Td mono muted className={cell}>
        {counts.map(c => c.value != null ? `${c.label}: ${c.value}` : null).filter(Boolean).join('  ·  ')}
        {error && (
          <details className="mt-0.5">
            <summary className="cursor-pointer text-danger text-[11px]">error</summary>
            <pre className="mt-1 whitespace-pre-wrap text-danger text-[11px]">{error}</pre>
          </details>
        )}
      </Td>
    </Tr>
  )
}

function RunStatus({ run }: { run: BatchRunLog }) {
  if (run.success) {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-success-bg text-success">
        <span className="size-1.5 rounded-full bg-success" />
        OK
      </span>
    )
  }
  if (run.completed_at) {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-danger-bg text-danger">
        <span className="size-1.5 rounded-full bg-danger" />
        Failed
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-accent-subtle text-accent">
      <span className="size-1.5 rounded-full bg-accent animate-pulse" />
      Running
    </span>
  )
}

function TriggerBadge({ source }: { source: 'cron' | 'manual' }) {
  if (source === 'manual') {
    return (
      <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-medium bg-warning-bg text-warning">
        manual
      </span>
    )
  }
  return (
    <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-medium bg-subtle text-muted">
      cron
    </span>
  )
}

function PhaseBreakdown({
  run,
  phase2Skipped,
  phase3Skipped,
}: {
  run: BatchRunLog
  phase2Skipped: boolean
  phase3Skipped: boolean
}) {
  return (
    <Table className="overflow-hidden">
      <Thead>
        <Th className="w-28">Phase</Th>
        <Th>Result</Th>
        <Th>Duration</Th>
        <Th>Counts</Th>
      </Thead>
      <Tbody>
        <PhaseRow
          label="1 · Sparse CF"
          ok={run.phase1_ok}
          durMs={run.phase1_duration_ms}
          counts={[
            { label: 'subjects', value: run.phase1_subjects },
            { label: 'objects', value: run.phase1_objects },
          ]}
          error={run.phase1_error}
        />
        <PhaseRow
          label="2 · Dense"
          ok={run.phase2_ok}
          durMs={run.phase2_duration_ms}
          counts={[
            { label: 'items', value: run.phase2_items },
            { label: 'subjects', value: run.phase2_subjects },
          ]}
          error={run.phase2_error}
          skipped={phase2Skipped}
        />
        <PhaseRow
          label="3 · Trending"
          ok={run.phase3_ok}
          durMs={run.phase3_duration_ms}
          counts={[{ label: 'items', value: run.phase3_items }]}
          error={run.phase3_error}
          skipped={phase3Skipped}
        />
      </Tbody>
    </Table>
  )
}

function LogViewer({ entries }: { entries: LogEntry[] }) {
  if (entries.length === 0) {
    return (
      <p className="text-[11px] text-muted italic px-3 py-2">No log entries captured.</p>
    )
  }
  return (
    <div className="font-mono text-[11px] leading-5 max-h-56 overflow-y-auto">
      {entries.map((e, i) => {
        const color =
          e.level === 'error' ? 'text-danger' :
          e.level === 'warn'  ? 'text-warning' :
          'text-secondary'
        const levelTag =
          e.level === 'error' ? 'ERR' :
          e.level === 'warn'  ? 'WRN' :
          'INF'
        const ts = e.ts.slice(11, 23) // HH:MM:SS.mmm
        return (
          <div key={i} className={`flex gap-2 px-3 py-0.5 hover:bg-surface-raised ${color}`}>
            <span className="text-muted shrink-0">{ts}</span>
            <span className="shrink-0 w-7">{levelTag}</span>
            <span className="break-all">{e.msg}</span>
          </div>
        )
      })}
    </div>
  )
}

function BatchRunModal({ run, onClose }: { run: BatchRunLog; onClose: () => void }) {
  const phase2Skipped = run.phase2_ok == null && run.phase1_ok != null
  const phase3Skipped = run.phase3_ok == null && run.phase1_ok != null
  const retrigger = useTriggerBatch(run.namespace)

  async function handleRetrigger() {
    try {
      await retrigger.mutateAsync()
      onClose()
    } catch {
      // error left visible via retrigger.isError
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={
        <span>
          Batch Run <span className="text-accent">#{run.id}</span>
        </span>
      }
    >
      <div className="flex flex-col gap-5">
        <div className="grid grid-cols-2 gap-x-6 gap-y-2 text-xs">
          <div className="text-muted">Namespace</div>
          <div><CodeBadge>{run.namespace}</CodeBadge></div>

          <div className="text-muted">Trigger</div>
          <div><TriggerBadge source={run.trigger_source} /></div>

          <div className="text-muted">Started</div>
          <div className="text-secondary tabular-nums">{fmtDate(run.started_at)}</div>

          <div className="text-muted">Completed</div>
          <div className="text-secondary tabular-nums">
            {run.completed_at
              ? fmtDate(run.completed_at)
              : <em className="not-italic text-accent">in progress</em>}
          </div>

          <div className="text-muted">Duration</div>
          <div className="text-secondary tabular-nums">
            {run.duration_ms != null
              ? `${run.duration_ms} ms`
              : run.completed_at
                ? '–'
                : <em className="not-italic text-accent">in progress</em>}
          </div>

          <div className="text-muted">Subjects processed</div>
          <div className="text-secondary tabular-nums">{run.subjects_processed ?? '—'}</div>

          <div className="text-muted">Status</div>
          <div><RunStatus run={run} /></div>

          {!run.success && run.completed_at && run.error_message && (
            <>
              <div className="text-muted">Error</div>
              <pre className="whitespace-pre-wrap text-danger text-[11px] m-0">{run.error_message}</pre>
            </>
          )}
        </div>

        {(run.phase1_ok != null || run.phase2_ok != null || run.phase3_ok != null) && (
          <div>
            <p className="text-[11px] font-semibold text-muted uppercase tracking-wide mb-2 m-0">Phase breakdown</p>
            <div className="border border-default rounded overflow-hidden py-2">
              <PhaseBreakdown run={run} phase2Skipped={phase2Skipped} phase3Skipped={phase3Skipped} />
            </div>
          </div>
        )}

        <div>
          <p className="text-[11px] font-semibold text-muted uppercase tracking-wide mb-2 m-0">
            Run log
            <span className="ml-2 normal-case font-normal text-muted">({run.log_lines.length} entries)</span>
          </p>
          <div className="border border-default rounded bg-canvas overflow-hidden">
            <LogViewer entries={run.log_lines} />
          </div>
        </div>

        <div className="flex items-center justify-between gap-3 pt-1 border-t border-default">
          <Link
            to={`/namespaces/${run.namespace}`}
            onClick={onClose}
            className="no-underline text-xs font-medium text-accent hover:text-accent-hover transition-colors"
          >
            View vector stats for {run.namespace} →
          </Link>
          <div className="flex items-center gap-2">
            {retrigger.isError && (
              <span className="text-xs text-danger">Failed to trigger</span>
            )}
            <Button
              size="sm"
              onClick={handleRetrigger}
              disabled={retrigger.isPending}
            >
              {retrigger.isPending ? 'Running…' : 'Re-run'}
            </Button>
          </div>
        </div>
      </div>
    </Modal>
  )
}

// ─── summary strip ────────────────────────────────────────────────────────────

function SummaryStrip({ runs, total }: { runs: BatchRunLog[]; total: number }) {
  const completed = runs.filter(r => r.completed_at)
  const failed = completed.filter(r => !r.success)
  const running = runs.filter(r => !r.completed_at)
  const failRate = completed.length > 0 ? Math.round((failed.length / completed.length) * 100) : null
  const lastFail = failed[0]

  return (
    <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-4">
      <SummaryCard label="Total runs" value={String(total)} />
      <SummaryCard
        label={`Fail rate (last ${completed.length})`}
        value={failRate != null ? `${failRate}%` : '—'}
        danger={failRate != null && failRate > 0}
      />
      <SummaryCard
        label="Currently running"
        value={running.length > 0 ? String(running.length) : '0'}
        live={running.length > 0}
      />
      <SummaryCard
        label="Last failure"
        value={lastFail ? `#${lastFail.id} · ${lastFail.namespace}` : 'None'}
        sub={lastFail ? fmtDateShort(lastFail.started_at) : undefined}
        danger={!!lastFail}
      />
    </div>
  )
}

function SummaryCard({
  label,
  value,
  sub,
  danger,
  live,
}: {
  label: string
  value: string
  sub?: string
  danger?: boolean
  live?: boolean
}) {
  return (
    <div className="bg-surface border border-default rounded px-4 py-3 flex flex-col gap-0.5">
      <div className="flex items-center gap-1.5">
        {live && <span className="size-1.5 rounded-full bg-accent animate-pulse" />}
        <span className="text-[11px] text-muted uppercase tracking-wide">{label}</span>
      </div>
      <span className={['text-sm font-semibold tabular-nums', danger ? 'text-danger' : 'text-primary'].join(' ')}>
        {value}
      </span>
      {sub && <span className="text-[11px] text-muted tabular-nums">{sub}</span>}
    </div>
  )
}

// ─── filter chips ─────────────────────────────────────────────────────────────

type StatusFilter = 'all' | 'running' | 'ok' | 'failed'

function FilterChips({
  value,
  onChange,
  counts,
}: {
  value: StatusFilter
  onChange: (v: StatusFilter) => void
  counts: Record<StatusFilter, number>
}) {
  const chips: { key: StatusFilter; label: string }[] = [
    { key: 'all', label: 'All' },
    { key: 'running', label: 'Running' },
    { key: 'ok', label: 'OK' },
    { key: 'failed', label: 'Failed' },
  ]
  return (
    <div className="flex gap-1.5 mb-3">
      {chips.map(({ key, label }) => (
        <button
          key={key}
          onClick={() => onChange(key)}
          className={[
            'px-2.5 py-1 cursor-pointer rounded text-xs font-medium transition-colors',
            value === key
              ? 'bg-accent text-accent-text'
              : 'bg-surface-raised text-secondary hover:text-primary',
          ].join(' ')}
        >
          {label}
          {counts[key] > 0 && (
            <span className={['ml-1.5 tabular-nums', value === key ? 'opacity-80' : 'text-muted'].join(' ')}>
              {counts[key]}
            </span>
          )}
        </button>
      ))}
    </div>
  )
}

// ─── page ─────────────────────────────────────────────────────────────────────

export default function BatchRunsPage() {
  const { namespace } = useActiveNamespace()
  const [selectedRun, setSelectedRun] = useState<BatchRunLog | null>(null)
  const [page, setPage] = useState(0)
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [lastNamespace, setLastNamespace] = useState(namespace)
  if (namespace !== lastNamespace) {
    setLastNamespace(namespace)
    setPage(0)
    setStatusFilter('all')
  }

  const { data, error, isLoading } = useBatchRuns(namespace || undefined, page, statusFilter === 'all' ? '' : statusFilter)
  const runNow = useTriggerBatch(namespace ?? '')

  const runs = data?.runs ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / BATCH_PAGE_SIZE)
  const hasRunning = (data?.stats.running ?? 0) > 0

  const counts: Record<StatusFilter, number> = {
    all: data?.stats.total ?? 0,
    running: data?.stats.running ?? 0,
    ok: data?.stats.ok ?? 0,
    failed: data?.stats.failed ?? 0,
  }

  return (
    <div>
      <PageHeader
        title="Batch Runs"
        actions={
          <div className="flex items-center gap-3">
            {hasRunning && (
              <span className="inline-flex items-center gap-1.5 text-xs text-accent">
                <span className="size-1.5 rounded-full bg-accent animate-pulse" />
                Live
              </span>
            )}
            {namespace && (
              <Button
                size="sm"
                onClick={() => runNow.mutate()}
                disabled={runNow.isPending}
              >
                {runNow.isPending ? 'Running…' : 'Run now'}
              </Button>
            )}
          </div>
        }
      />

      {error && <ErrorBanner message="Failed to load batch runs." />}
      {isLoading && <p className="text-sm text-muted">Loading…</p>}

      {data && runs.length === 0 && (
        <EmptyState>
          No runs yet — run{' '}
          <CodeBadge>make run-cron</CodeBadge>{' '}
          to populate batch history.
        </EmptyState>
      )}

      {data && total > 0 && (
        <>
          <SummaryStrip runs={runs} total={total} />

          <div>
            <FilterChips value={statusFilter} onChange={(v) => { setStatusFilter(v); setPage(0) }} counts={counts} />
          </div>
          <Panel>
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
                    onClick={() => setSelectedRun(run)}
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
              <div className="flex items-center justify-between px-2 pt-3 border-t border-default">
                <span className="text-xs text-muted">
                  {page * BATCH_PAGE_SIZE + 1}–{Math.min((page + 1) * BATCH_PAGE_SIZE, total)} of {total}
                </span>
                <div className="flex gap-1">
                  <button
                    disabled={page === 0}
                    onClick={() => setPage(p => p - 1)}
                    className="px-2.5 py-1 text-xs rounded text-secondary hover:text-primary hover:bg-surface-raised disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                  >
                    ← Prev
                  </button>
                  <button
                    disabled={page >= totalPages - 1}
                    onClick={() => setPage(p => p + 1)}
                    className="px-2.5 py-1 text-xs rounded text-secondary hover:text-primary hover:bg-surface-raised disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                  >
                    Next →
                  </button>
                </div>
              </div>
            )}
          </Panel>
        </>
      )}

      {selectedRun && (
        <BatchRunModal run={selectedRun} onClose={() => setSelectedRun(null)} />
      )}
    </div>
  )
}
