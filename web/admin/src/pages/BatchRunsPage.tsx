import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useBatchRuns } from '../hooks/useBatchRuns'
import { useNamespaceList } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

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
        <td className="px-3 py-1.5 text-[11px] text-primary w-[110px]">{label}</td>
        <td colSpan={3} className="px-3 py-1.5 text-xs text-muted italic">skipped</td>
      </tr>
    )
  }
  if (ok == null) {
    return (
      <tr className="border-t border-default">
        <td className="px-3 py-1.5 text-[11px] text-primary w-[110px]">{label}</td>
        <td colSpan={3} className="px-3 py-1.5 text-xs text-muted italic">no data</td>
      </tr>
    )
  }
  return (
    <tr className="border-t border-default">
      <td className="px-3 py-1.5 text-[11px] text-primary w-[110px]">{label}</td>
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
  const { data: nsData } = useNamespaceList()
  const [nsFilter, setNsFilter] = useState('')
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set())
  const { data, error, isLoading } = useBatchRuns(nsFilter || undefined)

  function toggleRow(id: number) {
    setExpandedRows(prev => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  return (
    <div>
      <div className="flex justify-between items-center mb-8">
        <h2 className="text-[28px] font-semibold text-primary -tracking-[0.01em] leading-tight m-0">
          Batch Runs
        </h2>
        <select
          value={nsFilter}
          onChange={e => setNsFilter(e.target.value)}
          className="bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary text-sm px-3 py-2 rounded-md focus:outline-none transition-shadow duration-100"
        >
          <option value="">All namespaces</option>
          {nsData?.namespaces.map(ns => (
            <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
          ))}
        </select>
      </div>

      {error && <ErrorBanner message="Failed to load batch runs." />}
      {isLoading && <p className="text-sm text-muted">Loading…</p>}

      {data && data.runs.length === 0 && (
        <div className="p-10 text-center text-sm text-muted border border-dashed border-default rounded-lg">
          No runs yet — run{' '}
          <code className="font-mono text-[12px] bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm">
            make run-cron
          </code>{' '}
          to populate batch history.
        </div>
      )}

      {data && data.runs.length > 0 && (
        <div className="bg-surface border border-default rounded-lg overflow-hidden">
          <table className="w-full border-collapse">
            <thead>
              <tr className="bg-subtle border-b-2 border-default">
                <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted w-8"></th>
                <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">ID</th>
                <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Namespace</th>
                <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Started</th>
                <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Duration</th>
                <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Subjects</th>
                <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Status</th>
                <th className="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.06em] text-muted"></th>
              </tr>
            </thead>
            <tbody>
              {data.runs.map(run => {
                const hasPhases = run.phase1_ok != null || run.phase2_ok != null || run.phase3_ok != null
                const expanded = expandedRows.has(run.id)
                const phase2Skipped = run.phase2_ok == null && run.phase1_ok != null
                const phase3Skipped = run.phase3_ok == null && run.phase1_ok != null

                return (
                  <>
                    <tr key={run.id} className="border-b border-default hover:bg-surface-raised">
                      <td className="px-4 py-3">
                        {hasPhases && (
                          <button
                            onClick={() => toggleRow(run.id)}
                            className={`cursor-pointer bg-transparent border-0 text-xs w-5 text-center p-0 transition-colors ${expanded ? 'text-accent' : 'text-muted'}`}
                            title="Toggle phase breakdown"
                          >
                            {expanded ? '▾' : '▸'}
                          </button>
                        )}
                      </td>
                      <td className="px-4 py-3 text-sm text-primary font-mono tabular-nums">{run.id}</td>
                      <td className="px-4 py-3 text-sm">
                        <code className="font-mono text-[12px] bg-accent-subtle text-accent px-1.5 py-0.5 rounded-sm font-medium">
                          {run.namespace}
                        </code>
                      </td>
                      <td className="px-4 py-3 text-sm text-primary font-mono tabular-nums">{new Date(run.started_at).toLocaleString()}</td>
                      <td className="px-4 py-3 text-sm text-primary font-mono tabular-nums">
                        {run.duration_ms != null
                          ? `${run.duration_ms} ms`
                          : run.completed_at
                            ? '–'
                            : <em className="text-muted not-italic text-accent">in progress</em>}
                      </td>
                      <td className="px-4 py-3 text-sm text-primary font-mono tabular-nums">{run.subjects_processed}</td>
                      <td className="px-4 py-3 text-sm">
                        {run.success ? (
                          <span className="text-success font-medium">✓ OK</span>
                        ) : run.completed_at ? (
                          <details>
                            <summary className="cursor-pointer text-danger font-medium">✗ Failed</summary>
                            <pre className="mt-1 whitespace-pre-wrap text-danger font-mono text-[11px]">{run.error_message}</pre>
                          </details>
                        ) : (
                          <span className="text-accent font-medium">⟳ Running</span>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <Link
                          to={`/namespaces/${run.namespace}`}
                          className="no-underline text-xs font-medium text-accent hover:text-accent-hover transition-colors"
                        >
                          vector stats →
                        </Link>
                      </td>
                    </tr>

                    {expanded && (
                      <tr key={`${run.id}-phases`} className="border-b border-default">
                        <td colSpan={8} className="px-5 py-3 bg-subtle">
                          <table className="w-full border-collapse border border-default rounded-lg overflow-hidden">
                            <thead>
                              <tr className="bg-surface border-b border-default">
                                <th className="px-3 py-1.5 text-left text-[11px] font-semibold text-muted w-[110px]">Phase</th>
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
                        </td>
                      </tr>
                    )}
                  </>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
