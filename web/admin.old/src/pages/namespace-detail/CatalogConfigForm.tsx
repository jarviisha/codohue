import { useMemo, useState } from 'react'
import {
  Button,
  FormControl,
  Notice,
  NumberInput,
  Panel,
  Select,
} from '../../components/ui'
import { useCatalogConfig, useUpdateCatalogConfig } from '../../hooks/useCatalogConfig'
import { ApiError } from '../../services/api'
import type {
  CatalogDimMismatchBody,
  CatalogStrategyDescriptor,
  NamespaceCatalogResponse,
  NamespaceCatalogUpdateRequest,
} from '../../types'

interface FormState {
  enabled: boolean
  strategy_id: string
  strategy_version: string
  max_attempts: number
  max_content_bytes: number
}

const DEFAULTS: FormState = {
  enabled: false,
  strategy_id: '',
  strategy_version: '',
  max_attempts: 5,
  max_content_bytes: 32768,
}

// CatalogConfigForm renders the per-namespace catalog auto-embedding config
// (T042). It owns both display ("get") and mutation ("put") of:
//
//   • enabled                     (toggle)
//   • strategy_id, strategy_version (selectors filtered by namespace embedding_dim)
//   • max_attempts                (number)
//   • max_content_bytes           (number, in bytes)
//
// The 400 dim-mismatch path (returned by PUT when the chosen strategy's
// natural Dim() differs from the namespace's embedding_dim) is rendered
// inline with both numbers from the API body.
export default function CatalogConfigForm({ namespace }: { namespace: string }) {
  const { data, isLoading, error: loadErr } = useCatalogConfig(namespace)

  if (loadErr instanceof ApiError && loadErr.status === 503) {
    return (
      <Panel title="Catalog Auto-Embedding">
        <Notice tone="warning">
          Catalog auto-embedding is not wired in this deployment.
        </Notice>
      </Panel>
    )
  }

  if (isLoading || !data) {
    return (
      <Panel title="Catalog Auto-Embedding">
        <p className="m-0 text-sm text-muted">Loading catalog config…</p>
      </Panel>
    )
  }

  // The keyed remount pattern: when the server-side catalog config changes
  // (after successful save → invalidate → refetch → new updated_at), React
  // remounts CatalogConfigFormBody with fresh `initial`. Avoids an effect-driven
  // hydration that would conflict with the eslint react-hooks/set-state-in-effect rule.
  return (
    <CatalogConfigFormBody
      key={data.catalog.updated_at}
      namespace={namespace}
      data={data}
    />
  )
}

function CatalogConfigFormBody({
  namespace,
  data,
}: {
  namespace: string
  data: NamespaceCatalogResponse
}) {
  const update = useUpdateCatalogConfig(namespace)
  const initialForm = useMemo(() => catalogFormFromResponse(data), [data])

  const [form, setForm] = useState<FormState>(initialForm)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [dimMismatch, setDimMismatch] = useState<CatalogDimMismatchBody | null>(null)
  const [savedAt, setSavedAt] = useState<Date | null>(null)
  const initialSignature = useMemo(() => catalogFormSignature(initialForm), [initialForm])
  const currentSignature = useMemo(() => catalogFormSignature(form), [form])
  const isDirty = currentSignature !== initialSignature

  // Available strategies are filtered by the API to those whose Dim matches
  // the namespace's embedding_dim. We further group by id for the picker.
  const groupedStrategies = useMemo(() => groupByID(data.available_strategies ?? []), [data])

  const versionOptions = useMemo(() => {
    if (!form.strategy_id) return []
    const versions = groupedStrategies.get(form.strategy_id) ?? []
    return versions.map(v => ({
      label: `${v.version} (dim ${v.dim})${v.description ? ` — ${v.description}` : ''}`,
      value: v.version,
    }))
  }, [form.strategy_id, groupedStrategies])

  // If the user toggles enabled=true without a strategy chosen, default to
  // the first registered (id, version). This avoids submitting empty strings.
  function handleEnableToggle(next: boolean) {
    setForm(prev => {
      if (!next) return { ...prev, enabled: false }
      const ids = Array.from(groupedStrategies.keys())
      const strategy_id = prev.strategy_id || ids[0] || ''
      const versions = groupedStrategies.get(strategy_id) ?? []
      const strategy_version = prev.strategy_version || versions[0]?.version || ''
      return { ...prev, enabled: true, strategy_id, strategy_version }
    })
  }

  function handleStrategyChange(id: string) {
    const versions = groupedStrategies.get(id) ?? []
    setForm(prev => ({
      ...prev,
      strategy_id: id,
      strategy_version: versions[0]?.version ?? '',
    }))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!isDirty) return
    setSaveError(null)
    setDimMismatch(null)

    const req: NamespaceCatalogUpdateRequest = {
      enabled: form.enabled,
      max_attempts: form.max_attempts,
      max_content_bytes: form.max_content_bytes,
    }
    if (form.enabled) {
      if (!form.strategy_id || !form.strategy_version) {
        setSaveError('Pick a strategy id and version before enabling.')
        return
      }
      req.strategy_id = form.strategy_id
      req.strategy_version = form.strategy_version
      // The chosen variant's dim is part of the descriptor identity. Backend
      // factories that support multiple dims under the same (id, version) —
      // e.g. internal-hashing-ngrams@v1 — require it as a Params entry. The
      // available_strategies list is already filtered server-side to variants
      // whose dim matches the namespace's embedding_dim, so the lookup yields
      // a single descriptor.
      const descriptor = (data.available_strategies ?? []).find(
        d => d.id === form.strategy_id && d.version === form.strategy_version,
      )
      if (descriptor) {
        req.params = { dim: descriptor.dim }
      }
    }

    try {
      await update.mutateAsync(req)
      setSavedAt(new Date())
    } catch (err) {
      if (err instanceof ApiError) {
        const dim = extractDimMismatch(err.body)
        if (dim != null) {
          setDimMismatch(dim)
          return
        }
        setSaveError(`${err.code}: ${err.message}`)
        return
      }
      setSaveError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  const noStrategiesAvailable = (data.available_strategies?.length ?? 0) === 0

  return (
    <Panel title="Catalog Auto-Embedding">
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        {dimMismatch && (
          <Notice tone="danger">
            Strategy dimension mismatch: chosen strategy returns{' '}
            <strong>{dimMismatch.strategy_dim}</strong> but the namespace's{' '}
            <code>embedding_dim</code> is <strong>{dimMismatch.namespace_embedding_dim}</strong>.
            Pick a different strategy or change the namespace's embedding dimension.
          </Notice>
        )}
        {saveError && (
          <Notice tone="danger" onDismiss={() => setSaveError(null)}>
            {saveError}
          </Notice>
        )}
        {savedAt && !saveError && !dimMismatch && (
          <Notice tone="success" onDismiss={() => setSavedAt(null)}>
            Saved at {savedAt.toLocaleTimeString()}.
          </Notice>
        )}

        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={form.enabled}
            onChange={e => handleEnableToggle(e.target.checked)}
            className="h-4 w-4"
          />
          <span className="text-sm font-medium text-primary">
            Enable catalog auto-embedding
          </span>
          <span className="text-xs text-muted">
            (clients can POST raw content to <code>/v1/namespaces/{namespace}/catalog</code>)
          </span>
        </label>

        {form.enabled && noStrategiesAvailable && (
          <Notice tone="warning">
            No registered strategy matches the namespace's <code>embedding_dim</code>{' '}
            ({data.catalog.embedding_dim}). Embedder must register a strategy whose{' '}
            <code>Dim()</code> equals the namespace's dim before catalog mode can be enabled.
          </Notice>
        )}

        {form.enabled && !noStrategiesAvailable && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <FormControl label="Strategy id" htmlFor="catalog-strategy-id">
              <Select
                id="catalog-strategy-id"
                value={form.strategy_id}
                onChange={e => handleStrategyChange(e.target.value)}
                options={Array.from(groupedStrategies.keys()).map(id => ({ label: id, value: id }))}
              />
            </FormControl>

            <FormControl label="Strategy version" htmlFor="catalog-strategy-version">
              <Select
                id="catalog-strategy-version"
                value={form.strategy_version}
                onChange={e => setForm(prev => ({ ...prev, strategy_version: e.target.value }))}
                options={versionOptions}
                disabled={!form.strategy_id}
              />
            </FormControl>

            <FormControl label="Max retry attempts" htmlFor="catalog-max-attempts">
              <NumberInput
                id="catalog-max-attempts"
                min={1}
                max={20}
                value={form.max_attempts}
                onChange={e =>
                  setForm(prev => ({ ...prev, max_attempts: Number(e.target.value) || 1 }))
                }
              />
            </FormControl>

            <FormControl label="Max content bytes" htmlFor="catalog-max-content-bytes">
              <NumberInput
                id="catalog-max-content-bytes"
                min={1024}
                step={1024}
                value={form.max_content_bytes}
                onChange={e =>
                  setForm(prev => ({
                    ...prev,
                    max_content_bytes: Number(e.target.value) || DEFAULTS.max_content_bytes,
                  }))
                }
              />
            </FormControl>
          </div>
        )}

        <div className="flex flex-col gap-3 border-t border-default pt-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <Button
              type="submit"
              variant="primary"
              disabled={update.isPending || !isDirty}
            >
              {update.isPending ? 'Saving…' : form.enabled ? 'Enable / save' : 'Disable / save'}
            </Button>
            <Button
              type="button"
              variant="ghost"
              disabled={!isDirty || update.isPending}
              onClick={() => setForm(initialForm)}
            >
              Reset
            </Button>
          </div>
          <div className="flex items-center gap-2">
            {isDirty && (
              <span className="text-xs font-medium text-warning">Unsaved catalog changes</span>
            )}
            <NamespaceDimHint dim={data.catalog.embedding_dim} />
          </div>
        </div>
      </form>
    </Panel>
  )
}

function catalogFormSignature(form: FormState) {
  return JSON.stringify(form)
}

function catalogFormFromResponse(data: NamespaceCatalogResponse): FormState {
  const c = data.catalog
  return {
    enabled: c.enabled,
    strategy_id: c.strategy_id ?? '',
    strategy_version: c.strategy_version ?? '',
    max_attempts: c.max_attempts || DEFAULTS.max_attempts,
    max_content_bytes: c.max_content_bytes || DEFAULTS.max_content_bytes,
  }
}

function NamespaceDimHint({ dim }: { dim: number }) {
  return (
    <span className="text-xs text-muted">
      Namespace <code>embedding_dim</code>: <strong>{dim}</strong>
    </span>
  )
}

// groupByID flattens descriptors into a strategy_id → versions[] map, in
// the order the API returned them. Used to drive the two-level picker.
function groupByID(
  descs: CatalogStrategyDescriptor[],
): Map<string, CatalogStrategyDescriptor[]> {
  const out = new Map<string, CatalogStrategyDescriptor[]>()
  for (const d of descs) {
    const list = out.get(d.id) ?? []
    list.push(d)
    out.set(d.id, list)
  }
  return out
}

// extractDimMismatch returns the typed dim-mismatch fields if the body
// matches the contract shape; null otherwise.
function extractDimMismatch(body: unknown): CatalogDimMismatchBody | null {
  if (!body || typeof body !== 'object') return null
  const b = body as Record<string, unknown>
  if (typeof b.error !== 'string') return null
  if (typeof b.strategy_dim !== 'number') return null
  if (typeof b.namespace_embedding_dim !== 'number') return null
  return {
    error: b.error,
    strategy_dim: b.strategy_dim,
    namespace_embedding_dim: b.namespace_embedding_dim,
  }
}

export type { NamespaceCatalogResponse }
