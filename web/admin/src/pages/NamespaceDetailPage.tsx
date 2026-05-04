import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useNamespace, useUpsertNamespace } from '../hooks/useNamespaces'
import { useQdrantStats } from '../hooks/useQdrantStats'
import type { QdrantCollectionStat } from '../hooks/useQdrantStats'
import ErrorBanner from '../components/ErrorBanner'

const defaultWeights: Record<string, number> = { VIEW: 1, LIKE: 5, COMMENT: 8, SHARE: 10, SKIP: -2 }

const inputClass = 'bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary text-sm px-3 py-2 rounded-md focus:outline-none transition-shadow duration-100 tabular-nums'

export default function NamespaceDetailPage() {
  const { ns } = useParams<{ ns: string }>()
  const isNew = !ns || ns === 'new'
  const navigate = useNavigate()

  const { data: existing, error: loadErr } = useNamespace(ns ?? '')
  const { data: qdrantStats } = useQdrantStats(isNew ? '' : (ns ?? ''))
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
    <div className="max-w-[560px]">
      <h2 className="text-[28px] font-semibold text-primary -tracking-[0.01em] leading-tight m-0 mb-8">
        {isNew ? 'Create Namespace' : `Edit: ${ns}`}
      </h2>

      {newKey && (
        <div className="p-5 mb-5 bg-success-bg border border-success/30 rounded-lg">
          <p className="text-sm font-medium text-primary m-0 mb-2">
            Namespace created. API key (shown once only):
          </p>
          <pre className="text-sm break-all m-0 mb-4 p-3 rounded-lg font-mono font-medium bg-accent-subtle text-accent border border-accent/20">
            {newKey}
          </pre>
          <button
            onClick={() => navigate('/namespaces')}
            className="text-sm font-medium text-accent-text bg-accent hover:bg-accent-hover rounded-md border-0 px-4 py-2 cursor-pointer transition-colors duration-150"
          >
            Done
          </button>
        </div>
      )}

      {saveError && <ErrorBanner message={saveError} onDismiss={() => setSaveError('')} />}

      {!isNew && qdrantStats && (
        <QdrantStatsPanel ns={ns!} stats={qdrantStats.collections} />
      )}

      {!newKey && (
        <form onSubmit={handleSave}>
          {isNew && (
            <Field label="Namespace name">
              <input
                required
                value={name}
                onChange={e => setName(e.target.value)}
                placeholder="e.g. my_feed"
                className={`${inputClass} w-full`}
              />
            </Field>
          )}

          <SectionHeader>Action Weights</SectionHeader>
          {Object.entries(weights).map(([k, v]) => (
            <Field key={k} label={k} inline>
              <input
                type="number"
                step="0.1"
                value={v}
                onChange={e => setWeights(w => ({ ...w, [k]: parseFloat(e.target.value) }))}
                className={`${inputClass} w-20`}
              />
            </Field>
          ))}

          <SectionHeader>Scoring Parameters</SectionHeader>
          <Field label="Lambda (time decay)" inline>
            <input type="number" step="0.001" value={lambda} onChange={e => setLambda(+e.target.value)} className={`${inputClass} w-24`} />
          </Field>
          <Field label="Gamma (object freshness)" inline>
            <input type="number" step="0.001" value={gamma} onChange={e => setGamma(+e.target.value)} className={`${inputClass} w-24`} />
          </Field>
          <Field label="Alpha (CF blend)" inline>
            <input type="number" step="0.01" min={0} max={1} value={alpha} onChange={e => setAlpha(+e.target.value)} className={`${inputClass} w-24`} />
          </Field>
          <Field label="Max results" inline>
            <input type="number" min={1} value={maxResults} onChange={e => setMaxResults(+e.target.value)} className={`${inputClass} w-24`} />
          </Field>
          <Field label="Seen items days" inline>
            <input type="number" min={1} value={seenDays} onChange={e => setSeenDays(+e.target.value)} className={`${inputClass} w-24`} />
          </Field>

          <SectionHeader>Dense Hybrid</SectionHeader>
          <Field label="Strategy">
            <select value={strategy} onChange={e => setStrategy(e.target.value)} className={`${inputClass} w-full`}>
              <option value="item2vec">item2vec</option>
              <option value="svd">svd</option>
              <option value="byoe">byoe</option>
              <option value="disabled">disabled</option>
            </select>
          </Field>
          <Field label="Embedding dim" inline>
            <input type="number" min={1} value={embDim} onChange={e => setEmbDim(+e.target.value)} className={`${inputClass} w-24`} />
          </Field>
          <Field label="Distance">
            <select value={distance} onChange={e => setDistance(e.target.value)} className={`${inputClass} w-full`}>
              <option value="cosine">cosine</option>
              <option value="dot">dot</option>
            </select>
          </Field>

          <SectionHeader>Trending</SectionHeader>
          <Field label="Window (hours)" inline>
            <input type="number" min={1} value={tWindow} onChange={e => setTWindow(+e.target.value)} className={`${inputClass} w-24`} />
          </Field>
          <Field label="TTL (seconds)" inline>
            <input type="number" min={0} value={tTTL} onChange={e => setTTTL(+e.target.value)} className={`${inputClass} w-24`} />
          </Field>
          <Field label="Lambda trending" inline>
            <input type="number" step="0.01" value={lambdaTrending} onChange={e => setLambdaTrending(+e.target.value)} className={`${inputClass} w-24`} />
          </Field>

          <div className="mt-6 flex gap-3">
            <button
              type="submit"
              disabled={upsert.isPending}
              className="bg-accent hover:bg-accent-hover active:bg-accent-active text-accent-text text-sm font-medium px-5 py-2.5 rounded-md border-0 cursor-pointer transition-colors duration-150 disabled:opacity-60 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:shadow-focus"
            >
              {upsert.isPending ? 'Saving…' : 'Save'}
            </button>
            <button
              type="button"
              onClick={() => navigate('/namespaces')}
              className="bg-transparent border border-default hover:border-strong hover:bg-surface-raised text-primary text-sm font-medium px-5 py-2.5 rounded-md cursor-pointer transition-colors duration-150 focus-visible:outline-none focus-visible:shadow-focus"
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
    <h3 className="font-semibold m-0 mt-6 mb-3 pb-2 text-[11px] uppercase tracking-[0.06em] text-muted border-b border-default">
      {children}
    </h3>
  )
}

function Field({ label, children, inline }: { label: string; children: React.ReactNode; inline?: boolean }) {
  return (
    <div className={`mb-3 ${inline ? 'flex items-center gap-4' : ''}`}>
      <label className={`text-[13px] font-medium text-primary ${inline ? 'min-w-[190px]' : 'block mb-1.5'}`}>
        {label}
      </label>
      {children}
    </div>
  )
}

function QdrantStatsPanel({ ns, stats }: { ns: string; stats: Record<string, QdrantCollectionStat> }) {
  const collections = [
    { key: `${ns}_subjects`, label: 'subjects (sparse)' },
    { key: `${ns}_objects`, label: 'objects (sparse)' },
    { key: `${ns}_subjects_dense`, label: 'subjects (dense)' },
    { key: `${ns}_objects_dense`, label: 'objects (dense)' },
  ]

  return (
    <div className="bg-surface border border-default rounded-lg p-5 mb-6">
      <h3 className="font-semibold m-0 mb-4 text-[11px] uppercase tracking-[0.06em] text-muted">
        Qdrant Collections
      </h3>
      <div className="grid grid-cols-4 gap-3">
        {collections.map(({ key, label }) => {
          const col = stats[key]
          return (
            <div
              key={key}
              className="flex flex-col p-3 bg-subtle border border-default rounded-lg"
            >
              <div className="text-xs text-muted mb-1 truncate" title={key}>{label}</div>
              {col?.exists ? (
                <>
                  <div className="text-[22px] font-bold text-primary tabular-nums -tracking-[0.02em]">
                    {col.points_count.toLocaleString()}
                  </div>
                  <div className="text-xs text-muted mt-0.5">pts</div>
                </>
              ) : (
                <div className="text-sm text-muted mt-1">—</div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
