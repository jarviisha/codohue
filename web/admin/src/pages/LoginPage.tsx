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
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: '#f5f5f5' }}>
      <form onSubmit={handleSubmit} style={{ background: '#fff', padding: '2rem', borderRadius: 8, boxShadow: '0 2px 8px rgba(0,0,0,0.1)', minWidth: 320 }}>
        <h1 style={{ marginTop: 0, fontSize: '1.5rem' }}>Codohue Admin</h1>
        {error && (
          <div style={{ background: '#ffeaea', color: '#c00', padding: '0.5rem 0.75rem', borderRadius: 4, marginBottom: '1rem', fontSize: '0.9rem' }}>
            {error}
          </div>
        )}
        <label style={{ display: 'block', marginBottom: '0.5rem', fontSize: '0.85rem', color: '#666' }}>
          API Key
        </label>
        <input
          type="password"
          value={apiKey}
          onChange={e => setApiKey(e.target.value)}
          placeholder="Enter RECOMMENDER_API_KEY"
          required
          style={{ width: '100%', padding: '0.5rem', borderRadius: 4, border: '1px solid #ccc', boxSizing: 'border-box', marginBottom: '1rem' }}
        />
        <button
          type="submit"
          disabled={loading}
          style={{ width: '100%', padding: '0.6rem', background: '#1a73e8', color: '#fff', border: 'none', borderRadius: 4, cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? 0.7 : 1 }}
        >
          {loading ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </div>
  )
}
