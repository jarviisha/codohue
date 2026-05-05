import { Fragment, useState } from 'react'
import { Link } from 'react-router-dom'
import { useBatchRuns } from '../hooks/useBatchRuns'
import ErrorBanner from '../components/ErrorBanner'
import { CodeBadge, EmptyState, PageHeader, Panel, Table, Thead, Th, Tbody, Tr, Td } from '../components/ui'
import type { BatchRunLog } from '../types'
import { useActiveNamespace } from '../context/NamespaceContext'

interface PhaseRowProps {
  label: string
  ok: boolean | null | undefined
  durMs: number | null | undefined
  counts: { label: string; value: number | null | undefined }[]
  error: string | null | undefined
  skipped?: boolean
}

function PhaseRow({ label, ok, durMs, counts, error, skipped }: PhaseRowProps) {
  if (skipped) {
    return (
      <tr className="border-t border-default">
        <td className="px-3 py-1.5 text-[11px] text-primary w-28">{label}</td>
        <td colSpan={3} className="px-3 py-1.5 text-xs text-muted italic">skipped</td>
      </tr>
    )
  }
  if (ok == null) {
    return (
      <tr className="border-t border-default">
        <td className="px-3 py-1.5 text-[11px] text-primary w-28">{label}</td>
        <td colSpan={3} className="px-3 py-1.5 text-xs text-muted italic">no data</td>
      </tr>
    )
  }
  return (
    <tr className="border-t border-default">
      <td className="px-3 py-1.5 text-[11px] text-primary w-28">{label}</td>
      <td className="px-3 py-1.5 text-xs font-medium">
        {ok
          ? <span className="text-success">✓ OK</span>
          : <span className="text-danger">✗ Failed</span>}
      </td>
      <td className="px-3 py-1.5 text-xs text-muted tabular-nums">
        {durMs != null ? `${durMs} ms` : '—'}
      </td>
      <td className="px-3 py-1.5 text-xs text-muted tabular-nums">
        {counts.map(c => c.value != null ? `${c.label}: ${c.value}` : null).filter(Boolean).join('  ·  ')}
        {error && (
          <details className="mt-0.5">
            <summary className="cursor-pointer text-danger text-[11px]">error</summary>
            <pre className="mt-1 whitespace-pre-wrap text-danger font-mono text-[11px]">{error}</pre>
          </details>
        )}
      </td>
    </tr>
  )
}

export default function BatchRunsPage() {
  const { namespace } = useActiveNamespace()
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set())
  const { data, error, isLoading } = useBatchRuns(namespace || undefined)

  function toggleRow(id: number) {
    setExpandedRows(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  return (
    <div>
      <PageHeader title="Batch Runs" />

      {error && <ErrorBanner message="Failed to load batch runs." />}
      {isLoading && <p className="text-sm text-muted">Loading…</p>}

      {data && data.runs.length === 0 && (
        <EmptyState>
          No runs yet — run{' '}
          <CodeBadge>make run-cron</CodeBadge>{' '}
          to populate batch history.
        </EmptyState>
      )}

      {data && data.runs.length > 0 && (
        <Panel>
          <Table>
            <Thead>
              <Th className="w-8" />
              <Th>ID</Th>
              <Th>Namespace</Th>
              <Th>Started</Th>
              <Th>Duration</Th>
              <Th>Subjects</Th>
              <Th>Status</Th>
              <Th />
            </Thead>
            <Tbody>
              {data.runs.map(run => {
                const hasPhases = run.phase1_ok != null || run.phase2_ok != null || run.phase3_ok != null
                const expanded = expandedRows.has(run.id)
                const phase2Skipped = run.phase2_ok == null && run.phase1_ok != null
                const phase3Skipped = run.phase3_ok == null && run.phase1_ok != null

                return (
                  <Fragment key={run.id}>
                    <Tr hoverable>
                      <Td>
                        {hasPhases && (
                          <button
                            onClick={() => toggleRow(run.id)}
                            className={`cursor-pointer bg-transparent border-0 text-xs w-5 text-center p-0 transition-colors ${expanded ? 'text-accent' : 'text-muted'}`}
                            title="Toggle phase breakdown"
                          >
                            {expanded ? '▾' : '▸'}
                          </button>
                        )}
                      </Td>
                      <Td mono>{run.id}</Td>
                      <Td>
                        <CodeBadge>{run.namespace}</CodeBadge>
                      </Td>
                      <Td mono>{new Date(run.started_at).toLocaleString()}</Td>
                      <Td mono>
                        {run.duration_ms != null
                          ? `${run.duration_ms} ms`
                          : run.completed_at
                            ? '–'
                            : <em className="not-italic text-accent">in progress</em>}
                      </Td>
                      <Td mono>{run.subjects_processed}</Td>
                      <Td>
                        <RunStatus run={run} />
                      </Td>
                      <Td>
                        <Link
                          to={`/namespaces/${run.namespace}`}
                          className="no-underline text-xs font-medium text-accent hover:text-accent-hover transition-colors"
                        >
                          vector stats →
                        </Link>
                      </Td>
                    </Tr>

                    {expanded && (
                      <tr className="border-b border-default last:border-0">
                        <td colSpan={8} className="py-3 bg-subtle">
                          <PhaseBreakdown
                            run={run}
                            phase2Skipped={phase2Skipped}
                            phase3Skipped={phase3Skipped}
                          />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                )
              })}
            </Tbody>
          </Table>
        </Panel>
      )}
    </div>
  )
}

function RunStatus({ run }: { run: BatchRunLog }) {
  if (run.success) {
    return <span className="text-success font-medium">✓ OK</span>
  }

  if (run.completed_at) {
    return (
      <details>
        <summary className="cursor-pointer text-danger font-medium">✗ Failed</summary>
        <pre className="mt-1 whitespace-pre-wrap text-danger font-mono text-[11px]">{run.error_message}</pre>
      </details>
    )
  }

  return <span className="text-accent font-medium">⟳ Running</span>
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
    <table className="w-full border-collapse border border-default rounded-xl overflow-hidden">
      <thead>
        <tr className="bg-surface border-b border-default">
          <th className="px-3 py-1.5 text-left text-[11px] font-semibold text-muted w-28">Phase</th>
          <th className="px-3 py-1.5 text-left text-[11px] font-semibold text-muted">Result</th>
          <th className="px-3 py-1.5 text-left text-[11px] font-semibold text-muted">Duration</th>
          <th className="px-3 py-1.5 text-left text-[11px] font-semibold text-muted">Counts</th>
        </tr>
      </thead>
      <tbody>
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
      </tbody>
    </table>
  )
}
