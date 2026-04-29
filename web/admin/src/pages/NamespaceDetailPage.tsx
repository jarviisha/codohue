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
    <div className="max-w-xl">
      <h2 className="mt-0 mb-4 text-xl font-semibold text-gray-800">
        {isNew ? 'Create Namespace' : `Edit: ${ns}`}
      </h2>

      {newKey && (
        <div role="alert" className="bg-green-50 border border-green-500 rounded-lg p-4 mb-4">
          <strong>Namespace created!</strong> API key (shown once only):
          <pre className="mt-2 font-mono text-sm break-all">{newKey}</pre>
          <button
            onClick={() => navigate('/namespaces')}
            className="mt-3 px-3 py-1.5 bg-blue-600 text-white border-none rounded cursor-pointer text-sm hover:bg-blue-700"
          >
            Done
          </button>
        </div>
      )}

      {saveError && <ErrorBanner message={saveError} onDismiss={() => setSaveError('')} />}

      {!newKey && (
        <form onSubmit={handleSave}>
          {isNew && (
            <Field label="Namespace name">
              <input required value={name} onChange={e => setName(e.target.value)} className={inputFull} placeholder="e.g. my_feed" />
            </Field>
          )}

          <SectionHeader>Action Weights</SectionHeader>
          {Object.entries(weights).map(([k, v]) => (
            <Field key={k} label={k} inline>
              <input type="number" step="0.1" value={v}
                onChange={e => setWeights(w => ({ ...w, [k]: parseFloat(e.target.value) }))}
                className={`${inputBase} w-20`} />
            </Field>
          ))}

          <SectionHeader>Scoring Parameters</SectionHeader>
          <Field label="Lambda (time decay)" inline><input type="number" step="0.001" value={lambda} onChange={e => setLambda(+e.target.value)} className={`${inputBase} w-24`} /></Field>
          <Field label="Gamma (object freshness)" inline><input type="number" step="0.001" value={gamma} onChange={e => setGamma(+e.target.value)} className={`${inputBase} w-24`} /></Field>
          <Field label="Alpha (CF blend)" inline><input type="number" step="0.01" min={0} max={1} value={alpha} onChange={e => setAlpha(+e.target.value)} className={`${inputBase} w-24`} /></Field>
          <Field label="Max results" inline><input type="number" min={1} value={maxResults} onChange={e => setMaxResults(+e.target.value)} className={`${inputBase} w-24`} /></Field>
          <Field label="Seen items days" inline><input type="number" min={1} value={seenDays} onChange={e => setSeenDays(+e.target.value)} className={`${inputBase} w-24`} /></Field>

          <SectionHeader>Dense Hybrid</SectionHeader>
          <Field label="Strategy">
            <select value={strategy} onChange={e => setStrategy(e.target.value)} className={inputFull}>
              <option value="item2vec">item2vec</option>
              <option value="svd">svd</option>
              <option value="byoe">byoe</option>
              <option value="disabled">disabled</option>
            </select>
          </Field>
          <Field label="Embedding dim" inline><input type="number" min={1} value={embDim} onChange={e => setEmbDim(+e.target.value)} className={`${inputBase} w-24`} /></Field>
          <Field label="Distance">
            <select value={distance} onChange={e => setDistance(e.target.value)} className={inputFull}>
              <option value="cosine">cosine</option>
              <option value="dot">dot</option>
            </select>
          </Field>

          <SectionHeader>Trending</SectionHeader>
          <Field label="Window (hours)" inline><input type="number" min={1} value={tWindow} onChange={e => setTWindow(+e.target.value)} className={`${inputBase} w-24`} /></Field>
          <Field label="TTL (seconds)" inline><input type="number" min={0} value={tTTL} onChange={e => setTTTL(+e.target.value)} className={`${inputBase} w-24`} /></Field>
          <Field label="Lambda trending" inline><input type="number" step="0.01" value={lambdaTrending} onChange={e => setLambdaTrending(+e.target.value)} className={`${inputBase} w-24`} /></Field>

          <div className="mt-6 flex gap-3">
            <button
              type="submit"
              disabled={upsert.isPending}
              className={`px-5 py-2 bg-blue-600 text-white border-none rounded text-sm font-medium ${
                upsert.isPending ? 'opacity-70 cursor-not-allowed' : 'cursor-pointer hover:bg-blue-700'
              }`}
            >
              {upsert.isPending ? 'Saving…' : 'Save'}
            </button>
            <button
              type="button"
              onClick={() => navigate('/namespaces')}
              className="px-5 py-2 bg-transparent border border-gray-300 rounded text-sm cursor-pointer hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  )
}

function SectionHeader({ children }: { children: React.ReactNode }) {
  return (
    <h3 className="text-sm font-semibold text-gray-700 border-b border-gray-200 pb-2 mt-5 mb-3">{children}</h3>
  )
}

function Field({ label, children, inline }: { label: string; children: React.ReactNode; inline?: boolean }) {
  return (
    <div className={`mb-3 ${inline ? 'flex items-center gap-4' : ''}`}>
      <label className={`text-sm text-gray-600 ${inline ? 'min-w-[180px]' : 'block mb-1'}`}>{label}</label>
      {children}
    </div>
  )
}

const inputBase = 'px-2.5 py-1.5 border border-gray-300 rounded text-sm'
const inputFull = `${inputBase} w-full`
