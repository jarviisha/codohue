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
    <div className="flex justify-center items-center min-h-screen bg-white">
      <div style={{ width: '360px' }}>
        <div className="mb-8 text-center">
          <h1
            className="font-light text-[#061b31] m-0 mb-2"
            style={{ fontSize: '32px', letterSpacing: '-0.64px', lineHeight: 1.1 }}
          >
            Codohue
            <span className="text-[#533afd] ml-2">Admin</span>
          </h1>
          <p className="text-sm text-[#64748d] font-light m-0">Sign in to your dashboard</p>
        </div>

        <form
          onSubmit={handleSubmit}
          className="bg-white p-8"
          style={{
            border: '1px solid #e5edf5',
            borderRadius: '6px',
            boxShadow: 'rgba(50,50,93,0.25) 0px 30px 45px -30px, rgba(0,0,0,0.1) 0px 18px 36px -18px',
          }}
        >
          {error && (
            <div
              className="px-4 py-3 mb-5 text-sm font-normal"
              style={{
                background: 'rgba(234,34,97,0.06)',
                border: '1px solid rgba(234,34,97,0.2)',
                borderRadius: '4px',
                color: '#ea2261',
              }}
            >
              {error}
            </div>
          )}

          <div className="mb-5">
            <label
              className="block mb-1.5 text-sm font-normal"
              style={{ color: '#273951' }}
            >
              API Key
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
              placeholder="Enter RECOMMENDER_API_KEY"
              required
              className="w-full py-2 px-3 text-sm font-normal outline-none transition-colors"
              style={{
                border: '1px solid #e5edf5',
                borderRadius: '4px',
                color: '#061b31',
              }}
              onFocus={e => { e.target.style.borderColor = '#533afd' }}
              onBlur={e => { e.target.style.borderColor = '#e5edf5' }}
            />
          </div>

          <button
            type="submit"
            disabled={loading}
            className="w-full py-2.5 text-sm font-normal text-white transition-colors"
            style={{
              background: loading ? '#4434d4' : '#533afd',
              borderRadius: '4px',
              border: 'none',
              cursor: loading ? 'not-allowed' : 'pointer',
              opacity: loading ? 0.8 : 1,
            }}
            onMouseEnter={e => { if (!loading) (e.target as HTMLElement).style.background = '#4434d4' }}
            onMouseLeave={e => { if (!loading) (e.target as HTMLElement).style.background = '#533afd' }}
          >
            {loading ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  )
}
