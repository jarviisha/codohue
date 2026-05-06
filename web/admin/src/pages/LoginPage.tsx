import { useState } from 'react'
import { login, ApiError } from '../services/api'
import { Button, FormControl, TextInput } from '../components/ui'

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
      <div className="w-90">
        <div className="mb-8 text-center">
          <h1 className="text-4xl font-bold text-primary leading-tight m-0 mb-2">
            Codohue
            <span className="text-accent ml-2 font-semibold">Admin</span>
          </h1>
          <p className="text-sm text-secondary m-0">Sign in to your dashboard</p>
        </div>

        <form
          onSubmit={handleSubmit}
          className="bg-surface border border-default rounded-lg p-8 shadow-overlay"
        >
          {error && (
            <div className="px-4 py-3 mb-5 text-sm font-medium rounded-lg bg-danger-bg border border-danger/25 text-danger">
              {error}
            </div>
          )}

          <FormControl label="API Key" htmlFor="login-api-key" className="mb-5">
            <TextInput
              id="login-api-key"
              type="password"
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
              placeholder="Enter RECOMMENDER_API_KEY"
              required
              className="w-full py-2.5"
            />
          </FormControl>

          <Button
            type="submit"
            variant="primary"
            disabled={loading}
            className="w-full"
          >
            {loading ? 'Signing in...' : 'Sign in'}
          </Button>
        </form>
      </div>
    </div>
  )
}
