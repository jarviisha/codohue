import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useNamespace, useUpsertNamespace } from '../hooks/useNamespaces'
import ErrorBanner from '../components/ErrorBanner'

const defaultWeights: Record<string, number> = { VIEW: 1, LIKE: 5, COMMENT: 8, SHARE: 10, SKIP: -2 }

export default function NamespaceDetailPage() {
  const { ns } = useParams<{ ns: string }>()
  const isNew = !ns || ns === 'new'
  const navigate = useNavigate()

  const { data: existing, error: loadErr } = useNamespace(ns ?? '')
  const upsert = useUpsertNamespace()

  const [name, setName] = useState('')
  const [weights, setWeights] = useState(defaultWeights)
  const [lambda, setLambda] = useState(0.05)
  const [gamma, setGamma] = useState(0.02)
  const [alpha, setAlpha] = useState(0.7)
  const [maxResults, setMaxResults] = useState(50)
  const [seenDays, setSeenDays] = useState(30)
  const [strategy, setStrategy] = useState('item2vec')
  const [embDim, setEmbDim] = useState(64)
  const [distance, setDistance] = useState('cosine')
  const [tWindow, setTWindow] = useState(24)
  const [tTTL, setTTTL] = useState(600)
  const [lambdaTrending, setLambdaTrending] = useState(0.1)
  const [newKey, setNewKey] = useState<string | null>(null)
  const [saveError, setSaveError] = useState('')

  useEffect(() => {
    if (existing) {
      setWeights(existing.action_weights || defaultWeights)
      setLambda(existing.lambda)
      setGamma(existing.gamma)
      setAlpha(existing.alpha)
      setMaxResults(existing.max_results)
      setSeenDays(existing.seen_items_days)
      setStrategy(existing.dense_strategy)
      setEmbDim(existing.embedding_dim)
      setDistance(existing.dense_distance)
      setTWindow(existing.trending_window)
      setTTTL(existing.trending_ttl)
      setLambdaTrending(existing.lambda_trending)
    }
  }, [existing])

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    setSaveError('')
    const nsName = isNew ? name : ns!
    try {
      const result = await upsert.mutateAsync({
        ns: nsName,
        payload: {
          action_weights: weights,
          lambda, gamma, alpha,
          max_results: maxResults,
          seen_items_days: seenDays,
          dense_strategy: strategy,
          embedding_dim: embDim,
          dense_distance: distance,
          trending_window: tWindow,
          trending_ttl: tTTL,
          lambda_trending: lambdaTrending,
        },
      })
      if (result.api_key) {
        setNewKey(result.api_key)
      } else {
        navigate('/namespaces')
      }
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  if (loadErr) return <ErrorBanner message="Failed to load namespace config." />

  return (
    <div style={{ maxWidth: 640 }}>
      <h2 style={{ marginTop: 0 }}>{isNew ? 'Create Namespace' : `Edit: ${ns}`}</h2>

      {newKey && (
        <div role="alert" style={{ background: '#e6f4ea', border: '1px solid #34a853', borderRadius: 8, padding: '1rem', marginBottom: '1rem' }}>
          <strong>Namespace created!</strong> API key (shown once only):
          <pre style={{ margin: '0.5rem 0 0', fontFamily: 'monospace', wordBreak: 'break-all' }}>{newKey}</pre>
          <button onClick={() => navigate('/namespaces')} style={{ marginTop: '0.75rem', padding: '0.4rem 0.75rem', background: '#1a73e8', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}>
            Done
          </button>
        </div>
      )}

      {saveError && <ErrorBanner message={saveError} onDismiss={() => setSaveError('')} />}

      {!newKey && (
        <form onSubmit={handleSave}>
          {isNew && (
            <Field label="Namespace name">
              <input required value={name} onChange={e => setName(e.target.value)} style={inputStyle} placeholder="e.g. my_feed" />
            </Field>
          )}

          <h3 style={{ fontSize: '1rem', borderBottom: '1px solid #e0e0e0', paddingBottom: '0.5rem' }}>Action Weights</h3>
          {Object.entries(weights).map(([k, v]) => (
            <Field key={k} label={k} inline>
              <input type="number" step="0.1" value={v}
                onChange={e => setWeights(w => ({ ...w, [k]: parseFloat(e.target.value) }))}
                style={{ ...inputStyle, width: 80 }} />
            </Field>
          ))}

          <h3 style={{ fontSize: '1rem', borderBottom: '1px solid #e0e0e0', paddingBottom: '0.5rem', marginTop: '1.25rem' }}>Scoring Parameters</h3>
          <Field label="Lambda (time decay)" inline><input type="number" step="0.001" value={lambda} onChange={e => setLambda(+e.target.value)} style={{ ...inputStyle, width: 100 }} /></Field>
          <Field label="Gamma (object freshness)" inline><input type="number" step="0.001" value={gamma} onChange={e => setGamma(+e.target.value)} style={{ ...inputStyle, width: 100 }} /></Field>
          <Field label="Alpha (CF blend)" inline><input type="number" step="0.01" min={0} max={1} value={alpha} onChange={e => setAlpha(+e.target.value)} style={{ ...inputStyle, width: 100 }} /></Field>
          <Field label="Max results" inline><input type="number" min={1} value={maxResults} onChange={e => setMaxResults(+e.target.value)} style={{ ...inputStyle, width: 100 }} /></Field>
          <Field label="Seen items days" inline><input type="number" min={1} value={seenDays} onChange={e => setSeenDays(+e.target.value)} style={{ ...inputStyle, width: 100 }} /></Field>

          <h3 style={{ fontSize: '1rem', borderBottom: '1px solid #e0e0e0', paddingBottom: '0.5rem', marginTop: '1.25rem' }}>Dense Hybrid</h3>
          <Field label="Strategy">
            <select value={strategy} onChange={e => setStrategy(e.target.value)} style={inputStyle}>
              <option value="item2vec">item2vec</option>
              <option value="svd">svd</option>
              <option value="byoe">byoe</option>
              <option value="disabled">disabled</option>
            </select>
          </Field>
          <Field label="Embedding dim" inline><input type="number" min={1} value={embDim} onChange={e => setEmbDim(+e.target.value)} style={{ ...inputStyle, width: 100 }} /></Field>
          <Field label="Distance">
            <select value={distance} onChange={e => setDistance(e.target.value)} style={inputStyle}>
              <option value="cosine">cosine</option>
              <option value="dot">dot</option>
            </select>
          </Field>

          <h3 style={{ fontSize: '1rem', borderBottom: '1px solid #e0e0e0', paddingBottom: '0.5rem', marginTop: '1.25rem' }}>Trending</h3>
          <Field label="Window (hours)" inline><input type="number" min={1} value={tWindow} onChange={e => setTWindow(+e.target.value)} style={{ ...inputStyle, width: 100 }} /></Field>
          <Field label="TTL (seconds)" inline><input type="number" min={0} value={tTTL} onChange={e => setTTTL(+e.target.value)} style={{ ...inputStyle, width: 100 }} /></Field>
          <Field label="Lambda trending" inline><input type="number" step="0.01" value={lambdaTrending} onChange={e => setLambdaTrending(+e.target.value)} style={{ ...inputStyle, width: 100 }} /></Field>

          <div style={{ marginTop: '1.5rem', display: 'flex', gap: '0.75rem' }}>
            <button type="submit" disabled={upsert.isPending} style={{ padding: '0.6rem 1.25rem', background: '#1a73e8', color: '#fff', border: 'none', borderRadius: 4, cursor: upsert.isPending ? 'not-allowed' : 'pointer' }}>
              {upsert.isPending ? 'Saving…' : 'Save'}
            </button>
            <button type="button" onClick={() => navigate('/namespaces')} style={{ padding: '0.6rem 1.25rem', background: 'none', border: '1px solid #ccc', borderRadius: 4, cursor: 'pointer' }}>
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  )
}

function Field({ label, children, inline }: { label: string; children: React.ReactNode; inline?: boolean }) {
  return (
    <div style={{ marginBottom: '0.75rem', display: inline ? 'flex' : 'block', alignItems: inline ? 'center' : undefined, gap: inline ? '1rem' : undefined }}>
      <label style={{ display: 'block', fontSize: '0.85rem', color: '#555', minWidth: inline ? 180 : undefined, marginBottom: inline ? 0 : '0.25rem' }}>{label}</label>
      {children}
    </div>
  )
}

const inputStyle: React.CSSProperties = { padding: '0.4rem 0.6rem', border: '1px solid #ccc', borderRadius: 4, fontSize: '0.9rem', width: '100%', boxSizing: 'border-box' }
