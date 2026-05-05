import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useNamespace, useUpsertNamespace } from '../hooks/useNamespaces'
import { useQdrantStats } from '../hooks/useQdrantStats'
import ErrorBanner from '../components/ErrorBanner'
import type { QdrantCollectionStat } from '../types'
import { Button, Field, PageHeader, Panel, inputClass } from '../components/ui'
import {
  defaultNamespaceForm,
  namespaceConfigToForm,
  namespaceFormToPayload,
  type NamespaceFormState,
} from './namespaceForm'

export default function NamespaceDetailPage() {
  const { ns } = useParams<{ ns: string }>()
  const isNew = !ns || ns === 'new'
  const navigate = useNavigate()

  const { data: existing, error: loadErr, isLoading } = useNamespace(ns ?? '')
  const { data: qdrantStats } = useQdrantStats(isNew ? '' : (ns ?? ''))
  const upsert = useUpsertNamespace()

  const [newKey, setNewKey] = useState<string | null>(null)
  const [saveError, setSaveError] = useState('')

  async function handleSave(form: NamespaceFormState) {
    setSaveError('')
    const nsName = isNew ? form.name : ns!
    try {
      const result = await upsert.mutateAsync({
        ns: nsName,
        payload: namespaceFormToPayload(form),
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

  const initialForm = isNew
    ? defaultNamespaceForm()
    : existing
      ? namespaceConfigToForm(existing)
      : null

  return (
    <div className="max-w-140">
      <PageHeader title={isNew ? 'Create Namespace' : `Edit: ${ns}`} />

      {newKey && (
        <CreatedApiKeyPanel apiKey={newKey} onDone={() => navigate('/namespaces')} />
      )}

      {saveError && <ErrorBanner message={saveError} onDismiss={() => setSaveError('')} />}

      {!isNew && qdrantStats && (
        <QdrantStatsPanel ns={ns!} stats={qdrantStats.collections} />
      )}

      {!newKey && isLoading && !initialForm && (
        <p className="text-sm text-muted">Loading namespace config…</p>
      )}

      {!newKey && initialForm && (
        <NamespaceForm
          key={isNew ? 'new' : existing?.updated_at ?? ns}
          initialForm={initialForm}
          isNew={isNew}
          isPending={upsert.isPending}
          onCancel={() => navigate('/namespaces')}
          onSubmit={handleSave}
        />
      )}
    </div>
  )
}

interface NamespaceFormProps {
  initialForm: NamespaceFormState
  isNew: boolean
  isPending: boolean
  onCancel: () => void
  onSubmit: (form: NamespaceFormState) => Promise<void>
}

function NamespaceForm({ initialForm, isNew, isPending, onCancel, onSubmit }: NamespaceFormProps) {
  const [form, setForm] = useState(initialForm)

  function update<K extends keyof NamespaceFormState>(field: K, value: NamespaceFormState[K]) {
    setForm(current => ({ ...current, [field]: value }))
  }

  function updateNumber(field: keyof NamespaceFormState, value: string) {
    setForm(current => ({ ...current, [field]: Number(value) }))
  }

  function updateWeight(action: string, value: string) {
    setForm(current => ({
      ...current,
      action_weights: {
        ...current.action_weights,
        [action]: parseFloat(value),
      },
    }))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    await onSubmit(form)
  }

  return (
    <form onSubmit={handleSubmit}>
      {isNew && (
        <Field label="Namespace name">
          <input
            required
            value={form.name}
            onChange={e => update('name', e.target.value)}
            placeholder="e.g. my_feed"
            className={`${inputClass} w-full`}
          />
        </Field>
      )}

      <SectionHeader>Action Weights</SectionHeader>
      {Object.entries(form.action_weights).map(([action, weight]) => (
        <Field key={action} label={action} inline>
          <input
            type="number"
            step="0.1"
            value={weight}
            onChange={e => updateWeight(action, e.target.value)}
            className={`${inputClass} w-20 tabular-nums`}
          />
        </Field>
      ))}

      <SectionHeader>Scoring Parameters</SectionHeader>
      <Field label="Lambda (time decay)" inline>
        <input type="number" step="0.001" value={form.lambda} onChange={e => updateNumber('lambda', e.target.value)} className={`${inputClass} w-24 tabular-nums`} />
      </Field>
      <Field label="Gamma (object freshness)" inline>
        <input type="number" step="0.001" value={form.gamma} onChange={e => updateNumber('gamma', e.target.value)} className={`${inputClass} w-24 tabular-nums`} />
      </Field>
      <Field label="Alpha (CF blend)" inline>
        <input type="number" step="0.01" min={0} max={1} value={form.alpha} onChange={e => updateNumber('alpha', e.target.value)} className={`${inputClass} w-24 tabular-nums`} />
      </Field>
      <Field label="Max results" inline>
        <input type="number" min={1} value={form.max_results} onChange={e => updateNumber('max_results', e.target.value)} className={`${inputClass} w-24 tabular-nums`} />
      </Field>
      <Field label="Seen items days" inline>
        <input type="number" min={1} value={form.seen_items_days} onChange={e => updateNumber('seen_items_days', e.target.value)} className={`${inputClass} w-24 tabular-nums`} />
      </Field>

      <SectionHeader>Dense Hybrid</SectionHeader>
      <Field label="Strategy">
        <select value={form.dense_strategy} onChange={e => update('dense_strategy', e.target.value)} className={`${inputClass} w-full`}>
          <option value="item2vec">item2vec</option>
          <option value="svd">svd</option>
          <option value="byoe">byoe</option>
          <option value="disabled">disabled</option>
        </select>
      </Field>
      <Field label="Embedding dim" inline>
        <input type="number" min={1} value={form.embedding_dim} onChange={e => updateNumber('embedding_dim', e.target.value)} className={`${inputClass} w-24 tabular-nums`} />
      </Field>
      <Field label="Distance">
        <select value={form.dense_distance} onChange={e => update('dense_distance', e.target.value)} className={`${inputClass} w-full`}>
          <option value="cosine">cosine</option>
          <option value="dot">dot</option>
        </select>
      </Field>

      <SectionHeader>Trending</SectionHeader>
      <Field label="Window (hours)" inline>
        <input type="number" min={1} value={form.trending_window} onChange={e => updateNumber('trending_window', e.target.value)} className={`${inputClass} w-24 tabular-nums`} />
      </Field>
      <Field label="TTL (seconds)" inline>
        <input type="number" min={0} value={form.trending_ttl} onChange={e => updateNumber('trending_ttl', e.target.value)} className={`${inputClass} w-24 tabular-nums`} />
      </Field>
      <Field label="Lambda trending" inline>
        <input type="number" step="0.01" value={form.lambda_trending} onChange={e => updateNumber('lambda_trending', e.target.value)} className={`${inputClass} w-24 tabular-nums`} />
      </Field>

      <div className="mt-6 flex gap-3">
        <Button type="submit" variant="primary" disabled={isPending}>
          {isPending ? 'Saving…' : 'Save'}
        </Button>
        <Button type="button" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}

function CreatedApiKeyPanel({ apiKey, onDone }: { apiKey: string; onDone: () => void }) {
  return (
    <Panel className="mb-5 bg-success-bg border-success/30">
      <p className="text-sm font-medium text-primary m-0 mb-2">
        Namespace created. API key (shown once only):
      </p>
      <pre className="text-sm break-all m-0 mb-4 p-3 rounded-xl font-medium bg-accent-subtle text-accent border border-accent/20">
        {apiKey}
      </pre>
      <Button variant="primary" onClick={onDone}>
        Done
      </Button>
    </Panel>
  )
}

function SectionHeader({ children }: { children: React.ReactNode }) {
  return (
    <h3 className="font-semibold m-0 mt-6 mb-3 pb-2 text-[11px] uppercase tracking-[0.06em] text-muted border-b border-default">
      {children}
    </h3>
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
    <Panel className="mb-6">
      <h3 className="font-semibold m-0 mb-4 text-[11px] uppercase tracking-[0.06em] text-muted">
        Qdrant Collections
      </h3>
      <div className="grid grid-cols-4 gap-3">
        {collections.map(({ key, label }) => {
          const col = stats[key]
          return (
            <div
              key={key}
              className="flex flex-col p-3 bg-subtle border border-default rounded-xl"
            >
              <div className="text-xs text-muted mb-1 truncate" title={key}>{label}</div>
              {col?.exists ? (
                <>
                  <div className="text-[22px] font-bold text-primary tabular-nums tracking-[-0.02em]">
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
    </Panel>
  )
}
