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
    <div className="flex justify-center items-center min-h-screen bg-base">
      <div className="w-[360px]">
        <div className="mb-8 text-center">
          <h1 className="text-4xl font-bold text-primary -tracking-[0.02em] leading-tight m-0 mb-2">
            Codohue
            <span className="text-accent ml-2 font-semibold">Admin</span>
          </h1>
          <p className="text-sm text-secondary m-0">Sign in to your dashboard</p>
        </div>

        <form
          onSubmit={handleSubmit}
          className="bg-surface border border-default rounded-xl p-8 shadow-overlay"
        >
          {error && (
            <div className="px-4 py-3 mb-5 text-sm font-medium rounded-lg bg-danger-bg border border-danger/25 text-danger">
              {error}
            </div>
          )}

          <div className="mb-5">
            <label className="block mb-1.5 text-[13px] font-medium text-primary">
              API Key
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
              placeholder="Enter RECOMMENDER_API_KEY"
              required
              className="w-full bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary placeholder:text-muted text-sm px-3 py-2.5 rounded-md focus:outline-none transition-shadow duration-100"
            />
          </div>

          <button
            type="submit"
            disabled={loading}
            className="w-full py-2.5 text-sm font-medium text-accent-text bg-accent hover:bg-accent-hover active:bg-accent-active rounded-md border-0 cursor-pointer transition-colors duration-150 disabled:opacity-60 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:shadow-focus"
          >
            {loading ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  )
}
