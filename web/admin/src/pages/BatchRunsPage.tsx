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
  const dimText = { color: '#64748d', fontSize: '12px', fontWeight: 300 }

  if (skipped) {
    return (
      <tr style={{ borderTop: '1px solid #e5edf5' }}>
        <td style={{ ...phaseTdStyle, color: '#273951' }}>{label}</td>
        <td colSpan={3} style={{ ...dimText, padding: '6px 12px', fontStyle: 'italic' }}>skipped</td>
      </tr>
    )
  }
  if (ok == null) {
    return (
      <tr style={{ borderTop: '1px solid #e5edf5' }}>
        <td style={{ ...phaseTdStyle, color: '#273951' }}>{label}</td>
        <td colSpan={3} style={{ ...dimText, padding: '6px 12px', fontStyle: 'italic' }}>no data</td>
      </tr>
    )
  }
  return (
    <tr style={{ borderTop: '1px solid #e5edf5' }}>
      <td style={{ ...phaseTdStyle, color: '#273951' }}>{label}</td>
      <td style={{ padding: '6px 12px', fontSize: '12px' }}>
        {ok
          ? <span style={{ color: '#108c3d', fontWeight: 400 }}>✓ OK</span>
          : <span style={{ color: '#ea2261', fontWeight: 400 }}>✗ Failed</span>}
      </td>
      <td style={{ ...dimText, padding: '6px 12px' }} className="tabular-nums">
        {durMs != null ? `${durMs} ms` : '—'}
      </td>
      <td style={{ ...dimText, padding: '6px 12px' }} className="tabular-nums">
        {counts.map(c => c.value != null ? `${c.label}: ${c.value}` : null).filter(Boolean).join('  ·  ')}
        {error && (
          <details className="mt-0.5">
            <summary className="cursor-pointer" style={{ color: '#ea2261', fontSize: '11px' }}>error</summary>
            <pre className="mt-1 whitespace-pre-wrap" style={{ color: '#ea2261', fontFamily: "'Source Code Pro', monospace", fontSize: '11px' }}>{error}</pre>
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
      <div className="flex justify-between items-center mb-6">
        <h2
          className="font-light text-[#061b31] m-0"
          style={{ fontSize: '26px', letterSpacing: '-0.26px', lineHeight: 1.12 }}
        >
          Batch Runs
        </h2>
        <select
          value={nsFilter}
          onChange={e => setNsFilter(e.target.value)}
          className="text-sm font-normal outline-none"
          style={{
            padding: '6px 12px',
            border: '1px solid #e5edf5',
            borderRadius: '4px',
            color: '#061b31',
            background: '#fff',
          }}
          onFocus={e => { e.target.style.borderColor = '#533afd' }}
          onBlur={e => { e.target.style.borderColor = '#e5edf5' }}
        >
          <option value="">All namespaces</option>
          {nsData?.namespaces.map(ns => (
            <option key={ns.namespace} value={ns.namespace}>{ns.namespace}</option>
          ))}
        </select>
      </div>

      {error && <ErrorBanner message="Failed to load batch runs." />}
      {isLoading && <p className="text-sm text-[#64748d] font-light">Loading…</p>}

      {data && data.runs.length === 0 && (
        <div
          className="p-10 text-center text-sm text-[#64748d] font-light"
          style={{ border: '1px dashed #d6d9fc', borderRadius: '6px' }}
        >
          No runs yet — run{' '}
          <code style={{ fontFamily: "'Source Code Pro', monospace", fontSize: '12px', background: '#f5f6ff', padding: '1px 6px', borderRadius: '3px', color: '#533afd' }}>
            make run-cron
          </code>{' '}
          to populate batch history.
        </div>
      )}

      {data && data.runs.length > 0 && (
        <div
          className="bg-white overflow-hidden"
          style={{ border: '1px solid #e5edf5', borderRadius: '6px', boxShadow: 'rgba(23,23,23,0.06) 0px 3px 6px' }}
        >
          <table className="w-full border-collapse">
            <thead>
              <tr style={{ borderBottom: '1px solid #e5edf5' }}>
                <th style={thStyle}></th>
                <th style={thStyle}>ID</th>
                <th style={thStyle}>Namespace</th>
                <th style={thStyle}>Started</th>
                <th style={thStyle}>Duration</th>
                <th style={thStyle}>Subjects</th>
                <th style={thStyle}>Status</th>
                <th style={thStyle}></th>
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
                    <tr key={run.id} style={{ borderBottom: '1px solid #e5edf5' }}>
                      <td style={tdStyle}>
                        {hasPhases && (
                          <button
                            onClick={() => toggleRow(run.id)}
                            className="cursor-pointer transition-colors"
                            style={{
                              background: 'transparent',
                              border: 'none',
                              color: expanded ? '#533afd' : '#64748d',
                              fontSize: '12px',
                              width: '20px',
                              textAlign: 'center',
                              padding: 0,
                            }}
                            title="Toggle phase breakdown"
                          >
                            {expanded ? '▾' : '▸'}
                          </button>
                        )}
                      </td>
                      <td style={{ ...tdStyle, ...monoStyle }} className="tabular-nums">{run.id}</td>
                      <td style={tdStyle}>
                        <code style={{ fontFamily: "'Source Code Pro', monospace", fontSize: '12px', background: '#f5f6ff', padding: '1px 6px', borderRadius: '3px', color: '#533afd', fontWeight: 500 }}>
                          {run.namespace}
                        </code>
                      </td>
                      <td style={{ ...tdStyle, ...monoStyle }} className="tabular-nums">{new Date(run.started_at).toLocaleString()}</td>
                      <td style={{ ...tdStyle, ...monoStyle }} className="tabular-nums">
                        {run.duration_ms != null
                          ? `${run.duration_ms} ms`
                          : run.completed_at
                            ? '–'
                            : <em style={{ color: '#64748d', fontStyle: 'italic' }}>in progress</em>}
                      </td>
                      <td style={{ ...tdStyle, ...monoStyle }} className="tabular-nums">{run.subjects_processed}</td>
                      <td style={tdStyle}>
                        {run.success ? (
                          <span style={{ color: '#108c3d', fontWeight: 400, fontSize: '13px' }}>✓ OK</span>
                        ) : run.completed_at ? (
                          <details>
                            <summary className="cursor-pointer" style={{ color: '#ea2261', fontWeight: 400, fontSize: '13px' }}>✗ Failed</summary>
                            <pre className="mt-1 whitespace-pre-wrap" style={{ color: '#ea2261', fontFamily: "'Source Code Pro', monospace", fontSize: '11px' }}>{run.error_message}</pre>
                          </details>
                        ) : (
                          <span style={{ color: '#533afd', fontSize: '13px' }}>⟳ Running</span>
                        )}
                      </td>
                      <td style={tdStyle}>
                        <Link
                          to={`/namespaces/${run.namespace}`}
                          className="no-underline text-xs transition-colors"
                          style={{ color: '#533afd' }}
                          onMouseEnter={e => { (e.currentTarget as HTMLElement).style.color = '#4434d4' }}
                          onMouseLeave={e => { (e.currentTarget as HTMLElement).style.color = '#533afd' }}
                        >
                          vector stats →
                        </Link>
                      </td>
                    </tr>

                    {expanded && (
                      <tr key={`${run.id}-phases`} style={{ borderBottom: '1px solid #e5edf5', background: '#fafbff' }}>
                        <td colSpan={8} style={{ padding: '8px 20px 12px' }}>
                          <table className="w-full border-collapse" style={{ border: '1px solid #e5edf5', borderRadius: '4px', overflow: 'hidden' }}>
                            <thead>
                              <tr style={{ background: '#f5f6ff', borderBottom: '1px solid #e5edf5' }}>
                                <th style={{ ...phaseTdStyle, color: '#273951', fontWeight: 400 }}>Phase</th>
                                <th style={{ padding: '5px 12px', textAlign: 'left', fontSize: '11px', color: '#273951', fontWeight: 400 }}>Result</th>
                                <th style={{ padding: '5px 12px', textAlign: 'left', fontSize: '11px', color: '#273951', fontWeight: 400 }}>Duration</th>
                                <th style={{ padding: '5px 12px', textAlign: 'left', fontSize: '11px', color: '#273951', fontWeight: 400 }}>Counts</th>
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

const thStyle: React.CSSProperties = {
  padding: '10px 16px',
  textAlign: 'left',
  fontSize: '12px',
  fontWeight: 400,
  color: '#64748d',
  letterSpacing: '0.02em',
}

const tdStyle: React.CSSProperties = {
  padding: '10px 16px',
  fontSize: '13px',
  color: '#273951',
  fontWeight: 300,
}

const monoStyle: React.CSSProperties = {
  fontFamily: "'Source Code Pro', monospace",
  fontWeight: 500,
  fontSize: '12px',
}

const phaseTdStyle: React.CSSProperties = {
  padding: '6px 12px',
  textAlign: 'left',
  fontSize: '11px',
  fontWeight: 400,
  width: '110px',
}
