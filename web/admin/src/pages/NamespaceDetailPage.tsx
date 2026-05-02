import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useNamespace, useUpsertNamespace } from '../hooks/useNamespaces'
import { useQdrantStats } from '../hooks/useQdrantStats'
import type { QdrantCollectionStat } from '../hooks/useQdrantStats'
import ErrorBanner from '../components/ErrorBanner'

const defaultWeights: Record<string, number> = { VIEW: 1, LIKE: 5, COMMENT: 8, SHARE: 10, SKIP: -2 }

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
    <div style={{ maxWidth: '560px' }}>
      <h2
        className="font-light text-[#061b31] m-0 mb-6"
        style={{ fontSize: '26px', letterSpacing: '-0.26px', lineHeight: 1.12 }}
      >
        {isNew ? 'Create Namespace' : `Edit: ${ns}`}
      </h2>

      {newKey && (
        <div
          className="p-5 mb-5"
          style={{
            background: 'rgba(21,190,83,0.05)',
            border: '1px solid rgba(21,190,83,0.3)',
            borderRadius: '5px',
          }}
        >
          <p className="text-sm font-normal text-[#061b31] m-0 mb-2">
            Namespace created. API key (shown once only):
          </p>
          <pre
            className="text-sm break-all m-0 mb-4 p-3 rounded"
            style={{
              fontFamily: "'Source Code Pro', monospace",
              fontWeight: 500,
              background: '#f5f6ff',
              color: '#533afd',
              border: '1px solid rgba(83,58,253,0.15)',
              borderRadius: '4px',
            }}
          >
            {newKey}
          </pre>
          <button
            onClick={() => navigate('/namespaces')}
            className="text-sm font-normal text-white cursor-pointer"
            style={{ background: '#533afd', border: 'none', borderRadius: '4px', padding: '7px 16px' }}
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
                style={inputFullStyle}
                onFocus={e => { e.target.style.borderColor = '#533afd' }}
                onBlur={e => { e.target.style.borderColor = '#e5edf5' }}
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
                style={{ ...inputBaseStyle, width: '80px' }}
                className="tabular-nums"
                onFocus={e => { e.target.style.borderColor = '#533afd' }}
                onBlur={e => { e.target.style.borderColor = '#e5edf5' }}
              />
            </Field>
          ))}

          <SectionHeader>Scoring Parameters</SectionHeader>
          <Field label="Lambda (time decay)" inline>
            <input type="number" step="0.001" value={lambda} onChange={e => setLambda(+e.target.value)} style={{ ...inputBaseStyle, width: '96px' }} className="tabular-nums" onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }} />
          </Field>
          <Field label="Gamma (object freshness)" inline>
            <input type="number" step="0.001" value={gamma} onChange={e => setGamma(+e.target.value)} style={{ ...inputBaseStyle, width: '96px' }} className="tabular-nums" onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }} />
          </Field>
          <Field label="Alpha (CF blend)" inline>
            <input type="number" step="0.01" min={0} max={1} value={alpha} onChange={e => setAlpha(+e.target.value)} style={{ ...inputBaseStyle, width: '96px' }} className="tabular-nums" onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }} />
          </Field>
          <Field label="Max results" inline>
            <input type="number" min={1} value={maxResults} onChange={e => setMaxResults(+e.target.value)} style={{ ...inputBaseStyle, width: '96px' }} className="tabular-nums" onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }} />
          </Field>
          <Field label="Seen items days" inline>
            <input type="number" min={1} value={seenDays} onChange={e => setSeenDays(+e.target.value)} style={{ ...inputBaseStyle, width: '96px' }} className="tabular-nums" onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }} />
          </Field>

          <SectionHeader>Dense Hybrid</SectionHeader>
          <Field label="Strategy">
            <select value={strategy} onChange={e => setStrategy(e.target.value)} style={inputFullStyle} onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }}>
              <option value="item2vec">item2vec</option>
              <option value="svd">svd</option>
              <option value="byoe">byoe</option>
              <option value="disabled">disabled</option>
            </select>
          </Field>
          <Field label="Embedding dim" inline>
            <input type="number" min={1} value={embDim} onChange={e => setEmbDim(+e.target.value)} style={{ ...inputBaseStyle, width: '96px' }} className="tabular-nums" onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }} />
          </Field>
          <Field label="Distance">
            <select value={distance} onChange={e => setDistance(e.target.value)} style={inputFullStyle} onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }}>
              <option value="cosine">cosine</option>
              <option value="dot">dot</option>
            </select>
          </Field>

          <SectionHeader>Trending</SectionHeader>
          <Field label="Window (hours)" inline>
            <input type="number" min={1} value={tWindow} onChange={e => setTWindow(+e.target.value)} style={{ ...inputBaseStyle, width: '96px' }} className="tabular-nums" onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }} />
          </Field>
          <Field label="TTL (seconds)" inline>
            <input type="number" min={0} value={tTTL} onChange={e => setTTTL(+e.target.value)} style={{ ...inputBaseStyle, width: '96px' }} className="tabular-nums" onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }} />
          </Field>
          <Field label="Lambda trending" inline>
            <input type="number" step="0.01" value={lambdaTrending} onChange={e => setLambdaTrending(+e.target.value)} style={{ ...inputBaseStyle, width: '96px' }} className="tabular-nums" onFocus={e => { e.target.style.borderColor = '#533afd' }} onBlur={e => { e.target.style.borderColor = '#e5edf5' }} />
          </Field>

          <div className="mt-6 flex gap-3">
            <button
              type="submit"
              disabled={upsert.isPending}
              className="text-sm font-normal text-white cursor-pointer transition-colors"
              style={{
                background: upsert.isPending ? '#4434d4' : '#533afd',
                border: 'none',
                borderRadius: '4px',
                padding: '8px 20px',
                opacity: upsert.isPending ? 0.8 : 1,
              }}
              onMouseEnter={e => { if (!upsert.isPending) (e.currentTarget as HTMLElement).style.background = '#4434d4' }}
              onMouseLeave={e => { if (!upsert.isPending) (e.currentTarget as HTMLElement).style.background = '#533afd' }}
            >
              {upsert.isPending ? 'Saving…' : 'Save'}
            </button>
            <button
              type="button"
              onClick={() => navigate('/namespaces')}
              className="text-sm font-normal cursor-pointer transition-colors"
              style={{
                background: 'transparent',
                border: '1px solid #b9b9f9',
                borderRadius: '4px',
                padding: '8px 20px',
                color: '#533afd',
              }}
              onMouseEnter={e => { (e.currentTarget as HTMLElement).style.background = 'rgba(83,58,253,0.05)' }}
              onMouseLeave={e => { (e.currentTarget as HTMLElement).style.background = 'transparent' }}
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
    <h3
      className="font-normal m-0 mt-6 mb-3 pb-2 text-xs uppercase tracking-widest"
      style={{ color: '#64748d', borderBottom: '1px solid #e5edf5', letterSpacing: '0.08em' }}
    >
      {children}
    </h3>
  )
}

function Field({ label, children, inline }: { label: string; children: React.ReactNode; inline?: boolean }) {
  return (
    <div className={`mb-3 ${inline ? 'flex items-center gap-4' : ''}`}>
      <label
        className={`text-sm font-normal ${inline ? 'min-w-[190px]' : 'block mb-1.5'}`}
        style={{ color: '#273951' }}
      >
        {label}
      </label>
      {children}
    </div>
  )
}

const inputBaseStyle: React.CSSProperties = {
  padding: '6px 10px',
  border: '1px solid #e5edf5',
  borderRadius: '4px',
  fontSize: '13px',
  color: '#061b31',
  fontWeight: 300,
  background: '#fff',
  outline: 'none',
  transition: 'border-color 0.15s',
}

const inputFullStyle: React.CSSProperties = {
  ...inputBaseStyle,
  width: '100%',
}

function QdrantStatsPanel({ ns, stats }: { ns: string; stats: Record<string, QdrantCollectionStat> }) {
  const collections = [
    { key: `${ns}_subjects`, label: 'subjects (sparse)' },
    { key: `${ns}_objects`, label: 'objects (sparse)' },
    { key: `${ns}_subjects_dense`, label: 'subjects (dense)' },
    { key: `${ns}_objects_dense`, label: 'objects (dense)' },
  ]

  return (
    <div
      className="bg-white p-5 mb-6"
      style={{ border: '1px solid #e5edf5', borderRadius: '6px', boxShadow: 'rgba(23,23,23,0.06) 0px 3px 6px' }}
    >
      <h3
        className="font-normal m-0 mb-4 text-xs uppercase tracking-widest"
        style={{ color: '#64748d', letterSpacing: '0.08em' }}
      >
        Qdrant Collections
      </h3>
      <div className="grid grid-cols-4 gap-3">
        {collections.map(({ key, label }) => {
          const col = stats[key]
          return (
            <div
              key={key}
              className="flex flex-col p-3"
              style={{ background: '#fafbff', border: '1px solid #e5edf5', borderRadius: '5px' }}
            >
              <div className="text-xs text-[#64748d] font-light mb-1 truncate" title={key}>{label}</div>
              {col?.exists ? (
                <>
                  <div
                    className="font-light text-[#061b31] tabular-nums"
                    style={{ fontSize: '22px', letterSpacing: '-0.3px' }}
                  >
                    {col.points_count.toLocaleString()}
                  </div>
                  <div className="text-xs text-[#64748d] font-light mt-0.5">pts</div>
                </>
              ) : (
                <div className="text-sm text-[#64748d] font-light mt-1">—</div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
