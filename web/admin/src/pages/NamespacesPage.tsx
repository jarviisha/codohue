import { useNavigate } from 'react-router-dom'
import { useNamespaceList } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

export default function NamespacesPage() {
  const { data, error, isLoading } = useNamespaceList()
  const navigate = useNavigate()

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h2 className="m-0 text-xl font-semibold text-gray-800">Namespaces</h2>
        <button
          onClick={() => navigate('/namespaces/new')}
          className="px-4 py-2 bg-blue-600 text-white border-none rounded cursor-pointer text-sm hover:bg-blue-700"
        >
          + Create Namespace
        </button>
      </div>

      {error && <ErrorBanner message="Failed to load namespaces." />}
      {isLoading && <p className="text-gray-400">Loading…</p>}

      {data && (
        <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
          <table className="w-full border-collapse">
            <thead>
              <tr className="bg-gray-50 border-b border-gray-200">
                <th className={th}>Namespace</th>
                <th className={th}>Strategy</th>
                <th className={th}>Max Results</th>
                <th className={th}>API Key</th>
                <th className={th}>Updated</th>
                <th className={th}></th>
              </tr>
            </thead>
            <tbody>
              {data.namespaces.length === 0 && (
                <tr>
                  <td colSpan={6} className="p-4 text-center text-gray-400">No namespaces yet</td>
                </tr>
              )}
              {data.namespaces.map(ns => (
                <tr key={ns.namespace} className="border-b border-gray-100">
                  <td className={td}><code className="font-mono text-sm bg-gray-100 px-1.5 py-0.5 rounded">{ns.namespace}</code></td>
                  <td className={td}>{ns.dense_strategy}</td>
                  <td className={td}>{ns.max_results}</td>
                  <td className={td}>{ns.has_api_key ? '✓' : '–'}</td>
                  <td className={td}>{new Date(ns.updated_at).toLocaleString()}</td>
                  <td className={td}>
                    <button
                      onClick={() => navigate(`/namespaces/${ns.namespace}`)}
                      className="bg-transparent border border-gray-300 rounded cursor-pointer px-2 py-1 text-sm hover:bg-gray-50"
                    >
                      Edit
                    </button>
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
