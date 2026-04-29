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
      <tr className="bg-gray-50">
        <td className={phaseTh}>{label}</td>
        <td colSpan={3} className="px-3 py-1 text-xs text-gray-400 italic">skipped</td>
      </tr>
    )
  }
  if (ok == null) {
    return (
      <tr className="bg-gray-50">
        <td className={phaseTh}>{label}</td>
        <td colSpan={3} className="px-3 py-1 text-xs text-gray-400 italic">no data</td>
      </tr>
    )
  }
  return (
    <tr className="bg-gray-50 border-t border-gray-100">
      <td className={phaseTh}>{label}</td>
      <td className="px-3 py-1 text-xs">
        {ok
          ? <span className="text-green-600 font-semibold">✓ OK</span>
          : <span className="text-red-500 font-semibold">✗ Failed</span>}
      </td>
      <td className="px-3 py-1 text-xs text-gray-500">
        {durMs != null ? `${durMs} ms` : '—'}
      </td>
      <td className="px-3 py-1 text-xs text-gray-600">
        {counts.map(c => c.value != null ? `${c.label}: ${c.value}` : null).filter(Boolean).join('  ·  ')}
        {error && (
          <details className="mt-0.5">
            <summary className="cursor-pointer text-red-400">error</summary>
            <pre className="mt-1 text-red-600 whitespace-pre-wrap">{error}</pre>
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
      <div className="flex justify-between items-center mb-4">
        <h2 className="m-0 text-xl font-semibold text-gray-800">Batch Runs</h2>
        <select
          value={nsFilter}
          onChange={e => setNsFilter(e.target.value)}
          className="px-2.5 py-1.5 border border-gray-300 rounded text-sm"
        >
          <option value="">All namespaces</option>
          {nsData?.namespaces.map(ns => (
            <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
          ))}
        </select>
      </div>

      {error && <ErrorBanner message="Failed to load batch runs." />}
      {isLoading && <p className="text-gray-400">Loading…</p>}

      {data && data.runs.length === 0 && (
        <div className="bg-white border border-gray-200 rounded-lg p-8 text-center text-gray-400">
          No runs yet — run <code className="font-mono text-sm bg-gray-100 px-1.5 py-0.5 rounded">make run-cron</code> to populate batch history.
        </div>
      )}

      {data && data.runs.length > 0 && (
        <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
          <table className="w-full border-collapse">
            <thead>
              <tr className="bg-gray-50 border-b border-gray-200">
                <th className={th}></th>
                <th className={th}>ID</th>
                <th className={th}>Namespace</th>
                <th className={th}>Started</th>
                <th className={th}>Duration</th>
                <th className={th}>Subjects</th>
                <th className={th}>Status</th>
                <th className={th}></th>
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
                    <tr key={run.id} className="border-b border-gray-100">
                      <td className={td}>
                        {hasPhases && (
                          <button
                            onClick={() => toggleRow(run.id)}
                            className="text-gray-400 hover:text-gray-600 w-5 text-center font-mono"
                            title="Toggle phase breakdown"
                          >
                            {expanded ? '▾' : '▸'}
                          </button>
                        )}
                      </td>
                      <td className={td}>{run.id}</td>
                      <td className={td}><code className="font-mono text-sm bg-gray-100 px-1.5 py-0.5 rounded">{run.namespace}</code></td>
                      <td className={td}>{new Date(run.started_at).toLocaleString()}</td>
                      <td className={td}>
                        {run.duration_ms != null
                          ? `${run.duration_ms} ms`
                          : run.completed_at
                            ? '–'
                            : <em className="text-gray-400">in progress</em>}
                      </td>
                      <td className={td}>{run.subjects_processed}</td>
                      <td className={td}>
                        {run.success ? (
                          <span className="text-green-600 font-semibold">✓ OK</span>
                        ) : run.completed_at ? (
                          <details>
                            <summary className="cursor-pointer text-red-500 font-semibold">✗ Failed</summary>
                            <pre className="mt-1 text-xs text-red-700 whitespace-pre-wrap">{run.error_message}</pre>
                          </details>
                        ) : (
                          <span className="text-yellow-500">⟳ Running</span>
                        )}
                      </td>
                      <td className={td}>
                        <Link
                          to={`/namespaces/${run.namespace}`}
                          className="text-xs text-blue-500 hover:text-blue-700 hover:underline whitespace-nowrap"
                        >
                          vector stats →
                        </Link>
                      </td>
                    </tr>

                    {expanded && (
                      <tr key={`${run.id}-phases`} className="border-b border-gray-100">
                        <td colSpan={8} className="px-6 pb-2 pt-0">
                          <table className="w-full text-xs border border-gray-100 rounded">
                            <thead>
                              <tr className="bg-gray-100 text-gray-500">
                                <th className={phaseTh}>Phase</th>
                                <th className="px-3 py-1 text-left font-semibold">Result</th>
                                <th className="px-3 py-1 text-left font-semibold">Duration</th>
                                <th className="px-3 py-1 text-left font-semibold">Counts</th>
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

const th = 'px-4 py-2.5 text-left text-sm font-semibold text-gray-500'
const td = 'px-4 py-2.5 text-sm text-gray-700'
const phaseTh = 'px-3 py-1 text-left font-semibold text-gray-600 w-28'
