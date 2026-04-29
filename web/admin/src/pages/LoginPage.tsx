import { useState } from 'react'
import { login, ApiError } from '../services/api'

export default function LoginPage() {
  const [apiKey, setApiKey] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await login(apiKey)
      window.location.href = '/'
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Login failed')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex justify-center items-center min-h-screen bg-gray-100">
      <form onSubmit={handleSubmit} className="bg-white p-8 rounded-lg shadow-md w-[320px]">
        <h1 className="mt-0 mb-6 text-2xl font-semibold text-gray-900">Codohue Admin</h1>
        {error && (
          <div className="bg-red-50 text-red-700 py-2 px-3 rounded mb-4 text-sm">
            {error}
          </div>
        )}
        <label className="block mb-1.5 text-sm text-gray-500">API Key</label>
        <input
          type="password"
          value={apiKey}
          onChange={e => setApiKey(e.target.value)}
          placeholder="Enter RECOMMENDER_API_KEY"
          required
          className="w-full py-2 px-3 rounded border border-gray-300 mb-4 text-sm"
        />
        <button
          type="submit"
          disabled={loading}
          className={`w-full py-2.5 bg-blue-600 text-white border-none rounded text-sm font-medium ${
            loading ? 'opacity-70 cursor-not-allowed' : 'cursor-pointer hover:bg-blue-700'
          }`}
        >
          {loading ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </div>
  )
}
