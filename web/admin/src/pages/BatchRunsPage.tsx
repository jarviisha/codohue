import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useBatchRuns } from '../hooks/useBatchRuns'
import { useNamespaceList } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

export default function BatchRunsPage() {
  const { data: nsData } = useNamespaceList()
  const [nsFilter, setNsFilter] = useState('')
  const { data, error, isLoading } = useBatchRuns(nsFilter || undefined)

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
              {data.runs.map(run => (
                <tr key={run.id} className="border-b border-gray-100">
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
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

const th = 'px-4 py-2.5 text-left text-sm font-semibold text-gray-500'
const td = 'px-4 py-2.5 text-sm text-gray-700'
