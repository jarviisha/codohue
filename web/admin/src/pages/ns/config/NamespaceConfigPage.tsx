import { useMemo, useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import {
  Badge,
  Banner,
  Button,
  IconButton,
  NumberInput,
  Selector,
  Skeleton,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  TextInput,
  useToast,
} from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import {
  useNamespaceDashboard,
  useUpsertNamespace,
  type NamespaceConfig,
  type NamespaceUpsertRequest,
} from '@/services/namespaces'
import { useCatalogStrategies } from '@/services/catalog'
import PageHeader from '@/components/shell/PageHeader'
import DirtyFormGuard from '@/components/shell/DirtyFormGuard'
import CatalogStrategyFields from '@/components/CatalogStrategyFields'

// The form lives in the page body while Save/Reset are portalled into the
// global header, i.e. outside the <form> element. `form={FORM_ID}` re-attaches
// the submit button across that DOM gap so the browser runs native constraint
// validation — calling handleSubmit from an onClick skips it entirely.
const FORM_ID = 'namespace-config-form'

const DENSE_SOURCES = [
  { value: 'disabled', label: 'disabled — sparse only' },
  { value: 'byoe', label: 'byoe — bring your own embeddings' },
  { value: 'item2vec', label: 'item2vec — cron retrains from events' },
  { value: 'svd', label: 'svd — cron retrains via matrix factorisation' },
  { value: 'catalog', label: 'catalog — auto-embed from ingested content' },
]

const DENSE_DISTANCES = [
  { value: 'cosine', label: 'cosine' },
  { value: 'dot', label: 'dot product' },
]

/**
 * NamespaceConfigPage edits the full namespace_configs row for one namespace.
 *
 * Mirrors internal/admin/types.go::NamespaceUpsertRequest: every field is a
 * pointer on the wire, so submit sends only the keys that differ from the
 * server's current snapshot. Action weights are diffed value-by-value so a
 * single renamed action doesn't replay the whole map.
 *
 * Catalog auto-embedding settings live in a separate dialog reachable from
 * /ns/:ns/catalog because they require dim-matching with strategy versions.
 */
export default function NamespaceConfigPage() {
  const { ns } = useParams<{ ns: string }>()
  const showToast = useToast()
  const dashboard = useNamespaceDashboard(ns ?? null)
  const upsert = useUpsertNamespace()

  if (!ns) return null

  if (dashboard.isLoading) {
    return (
      <PageContainer size="md">
        <Skeleton className="h-48 w-full" />
      </PageContainer>
    )
  }

  if (dashboard.isError || !dashboard.data) {
    return (
      <PageContainer size="md">
        <Banner
          status="error"
          title="Could not load namespace config"
          description={dashboard.error?.message ?? 'unknown error'}
        />
      </PageContainer>
    )
  }

  return (
    <ConfigForm
      key={dashboard.data.config.updated_at}
      ns={ns}
      initial={dashboard.data.config}
      authorCoverage={dashboard.data.author_coverage}
      onSubmit={(body, onReset) => {
        upsert.mutate(
          { namespace: ns, body },
          {
            onSuccess: () => {
              showToast({ body: `Configuration saved — ${ns} updated.` })
              onReset()
            },
          },
        )
      }}
      error={upsert.error?.message}
      saving={upsert.isPending}
    />
  )
}

function ConfigForm({
  ns,
  initial,
  authorCoverage,
  onSubmit,
  saving,
  error,
}: {
  ns: string
  initial: NamespaceConfig
  authorCoverage: { attributed: number; total: number }
  onSubmit: (body: NamespaceUpsertRequest, onReset: () => void) => void
  saving: boolean
  error: string | undefined
}) {
  const [draft, setDraft] = useState<NamespaceConfig>(initial)
  // Catalog strategy pickers, only used while switching *into* catalog mode.
  const [strategyId, setStrategyId] = useState('')
  const [strategyVersion, setStrategyVersion] = useState('')
  // Action weights kept as ordered entries so adds/removes don't reshuffle
  // existing rows on every keystroke. A freshly created namespace can come
  // back with action_weights = null, so coalesce before iterating.
  const [weights, setWeights] = useState<Array<{ name: string; value: number }>>(() =>
    Object.entries(initial.action_weights ?? {}).map(([name, value]) => ({ name, value })),
  )

  // Once a namespace is in catalog mode the upsert cannot move it out — the
  // repository pins dense_source='catalog' and leaving the mode belongs to the
  // catalog endpoint (disable). Sending it anyway returned 200 and the UI
  // toasted "saved" while the column never moved, so the picker is locked here
  // and the operator is pointed at the Catalog tab instead.
  const lockedInCatalog = initial.dense_source === 'catalog'
  const switchingToCatalog = !lockedInCatalog && draft.dense_source === 'catalog'

  const strategies = useCatalogStrategies(draft.embedding_dim, switchingToCatalog)
  const descriptors = useMemo(() => strategies.data?.strategies ?? [], [strategies.data])

  const catalogSelection = useMemo(
    () => (switchingToCatalog ? { id: strategyId, version: strategyVersion } : undefined),
    [switchingToCatalog, strategyId, strategyVersion],
  )

  const dirty = useMemo(
    () => diffConfig(initial, draft, weights, catalogSelection).dirty,
    [initial, draft, weights, catalogSelection],
  )

  const reset = () => {
    setDraft(initial)
    setStrategyId('')
    setStrategyVersion('')
    setWeights(
      Object.entries(initial.action_weights ?? {}).map(([name, value]) => ({ name, value })),
    )
  }

  // Blocks a submit the backend would reject: catalog mode needs its strategy
  // in the same request (422 otherwise).
  const catalogIncomplete = switchingToCatalog && (strategyId === '' || strategyVersion === '')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    const { body } = diffConfig(initial, draft, weights, catalogSelection)
    onSubmit(body, reset)
  }

  return (
    <PageContainer size="lg">
      <DirtyFormGuard dirty={dirty && !saving} />
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full" wrap="wrap">
          <Stack gap={1}>
            <Stack gap={4} direction="horizontal" align="center">
              <h1 className="text-primary text-xl font-semibold">Configuration</h1>
              {dirty && <Badge variant="warning" label="unsaved" />}
            </Stack>
            <p className="text-secondary text-sm">
              Mirrors PUT /api/admin/v1/namespaces/{ns}. Only changed fields are sent.
            </p>
          </Stack>
          <Stack gap={4} direction="horizontal" align="center">
            <Button
              type="button"
              variant="ghost"
              
              isDisabled={!dirty || saving}
              onClick={reset} label="Reset" />
            <Button
              type="submit"
              form={FORM_ID}
              isDisabled={!dirty || saving || catalogIncomplete} label={saving ? 'Saving…' : 'Save'} />
          </Stack>
        </Stack>
      </PageHeader>

      <form id={FORM_ID} onSubmit={handleSubmit}>
        <Stack gap={6}>
          {error && <Banner status="error" title="Save failed" description={error} />}

          <SectionCard
            title="Recommend behavior"
            description="Blend, decay, and seen-items rules applied at serve time."
          >
            <NumberInput
              min={0}
              max={1}
              step={0.01}
              value={draft.alpha}
              onChange={(next) => setDraft({ ...draft, alpha: next })}
              label="Alpha (sparse weight)"
              description="0 = dense only, 1 = sparse only. Hybrid only kicks in when alpha < 1 AND dense_source ≠ disabled."
            />
            <NumberInput
              min={0.001}
              step={0.001}
              value={draft.lambda}
              onChange={(next) => setDraft({ ...draft, lambda: next })}
              label="Lambda (time decay)"
              description="Higher = older events count less. Applied during sparse vector build. Must be > 0."
            />
            <NumberInput
              min={0}
              step={0.001}
              value={draft.gamma}
              onChange={(next) => setDraft({ ...draft, gamma: next })}
              label="Gamma (object freshness)"
              description="γ-based freshness rerank at serve time. 0 = disabled."
            />
            <NumberInput
              min={1}
              max={500}
              value={draft.max_results}
              onChange={(next) => setDraft({ ...draft, max_results: next })}
              label="Max results"
              description="Maximum recommendations the API will return for a single subject."
            />
            <NumberInput
              min={1}
              value={draft.seen_items_days}
              onChange={(next) => setDraft({ ...draft, seen_items_days: next })}
              label="Seen items days"
              description="Recency window for the seen-items filter — events older than this are not counted as 'already seen'."
            />
            <Switch
              value={draft.exclude_authored ?? false}
              onChange={(next) => setDraft({ ...draft, exclude_authored: next })}
              label={draft.exclude_authored ? 'on' : 'off'}
              description={excludeAuthoredHelp(authorCoverage)}
            />
          </SectionCard>

          <SectionCard
            title="Dense vectors"
            description="Source + shape of dense embeddings used in the hybrid blend."
          >
            <Selector
              label="Dense source"
              description={
                lockedInCatalog
                  ? 'This namespace is in catalog mode. Leaving catalog mode is owned by the catalog endpoint — use Disable on the Catalog tab; changing it here would be silently ignored.'
                  : 'Single producer of object dense vectors: disabled (sparse only), byoe (you push embeddings), item2vec / svd (cron retrains from events), or catalog (auto-embed ingested content — pick the strategy below).'
              }
              value={draft.dense_source}
              isDisabled={lockedInCatalog}
              onChange={(next) => {
                setDraft({ ...draft, dense_source: next })
                setStrategyId('')
                setStrategyVersion('')
              }}
              options={DENSE_SOURCES.map((s) => ({ value: s.value, label: s.label }))}
            />

            {switchingToCatalog && (
              <CatalogStrategyFields
                embeddingDim={draft.embedding_dim}
                descriptors={descriptors}
                loading={strategies.isLoading}
                error={strategies.error?.message}
                strategyId={strategyId}
                strategyVersion={strategyVersion}
                onStrategyId={(next) => {
                  setStrategyId(next)
                  setStrategyVersion(descriptors.find((s) => s.id === next)?.version ?? '')
                }}
                onStrategyVersion={setStrategyVersion}
              />
            )}
            <NumberInput
              min={8}
              max={2048}
              value={draft.embedding_dim}
              onChange={(next) => {
              setDraft({ ...draft, embedding_dim: next })
              // Strategies are listed per dim, so a dim change can
              // invalidate the current pick.
              setStrategyId('')
              setStrategyVersion('')
              }}
              label="Embedding dim"
              description="Width of dense vectors. Must match what your producer sends (BYOE) or what item2vec/svd is configured for."
            />
            <Selector
              label="Dense distance"
              description="Similarity metric for the dense Qdrant collections. cosine for normalized embeddings, dot product when magnitude carries signal."
              value={draft.dense_distance}
              onChange={(next) => setDraft({ ...draft, dense_distance: next })}
              options={DENSE_DISTANCES.map((d) => ({ value: d.value, label: d.label }))}
            />
          </SectionCard>

          <SectionCard
            title="Trending"
            description="Cold-start fallback path — Redis ZSET that drives /v1/trending and the hybrid for new subjects."
          >
            <NumberInput
              min={1}
              value={draft.trending_window}
              onChange={(next) => setDraft({ ...draft, trending_window: next })}
              label="Trending window (hours)"
              description="Events newer than this contribute to the trending score."
            />
            <NumberInput
              min={1}
              value={draft.trending_ttl}
              onChange={(next) => setDraft({ ...draft, trending_ttl: next })}
              label="Trending TTL (seconds)"
              description="How long the Redis ZSET is cached before recomputation triggers."
            />
            <NumberInput
              min={0.001}
              step={0.001}
              value={draft.lambda_trending}
              onChange={(next) => setDraft({ ...draft, lambda_trending: next })}
              label="Lambda trending"
              description="Time decay applied while building the trending ZSET. Independent of the sparse lambda. Must be > 0."
            />
          </SectionCard>

          <SectionCard
            title="Action weights"
            description="Per-action weight applied during sparse vector build. Higher = stronger signal."
          >
            <ActionWeightsEditor weights={weights} onChange={setWeights} />
          </SectionCard>
        </Stack>
      </form>
    </PageContainer>
  )
}

function SectionCard({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <Stack gap={6}>
      <Stack gap={6} className="border-border border-b-2 pb-3">
        <span className="text-primary text-2xl font-semibold tracking-wide">{title}</span>
        <p className="text-secondary text-sm">{description}</p>
      </Stack>
      {children}
    </Stack>
  )
}

function ActionWeightsEditor({
  weights,
  onChange,
}: {
  weights: Array<{ name: string; value: number }>
  onChange: (next: Array<{ name: string; value: number }>) => void
}) {
  const setRow = (idx: number, patch: Partial<{ name: string; value: number }>) => {
    onChange(weights.map((w, i) => (i === idx ? { ...w, ...patch } : w)))
  }
  const removeRow = (idx: number) => onChange(weights.filter((_, i) => i !== idx))
  const addRow = () => onChange([...weights, { name: '', value: 1 }])

  if (weights.length === 0) {
    return (
      <Stack gap={6}>
        <p className="text-secondary text-sm">No actions configured.</p>
        <Stack align="center" gap={4} direction="horizontal" justify="end">
          <Button type="button" size="sm" variant="secondary"  onClick={addRow} label="Add action" />
        </Stack>
      </Stack>
    )
  }

  return (
    <Stack gap={6}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHeaderCell>Action</TableHeaderCell>
              <TableHeaderCell className="text-right" >Weight</TableHeaderCell>
              <TableHeaderCell className="text-right" >Remove</TableHeaderCell>
            </TableRow>
          </TableHeader>
          <TableBody>
            {weights.map((w, i) => (
              <TableRow key={i}>
                <TableCell>
                  <TextInput
                    label="Action name"
                    isLabelHidden
                    value={w.name}
                    onChange={(next) => setRow(i, { name: next })}
                    placeholder="e.g. click"
                  />
                </TableCell>
                <TableCell className="text-right" >
                  <NumberInput
                    label="Weight"
                    isLabelHidden
                    step={0.01}
                    value={w.value}
                    onChange={(next) => setRow(i, { value: next })}
                  />
                </TableCell>
                <TableCell className="text-right" >
                  <IconButton
                    label={`Remove action ${w.name || i}`}
                    icon="×"
                    variant="destructive"
                    onClick={() => removeRow(i)}
                  />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      <Stack align="center" gap={4} direction="horizontal" justify="end">
        <Button type="button" size="sm" variant="secondary"  onClick={addRow} label="Add action" />
      </Stack>
    </Stack>
  )
}

function diffConfig(
  initial: NamespaceConfig,
  draft: NamespaceConfig,
  weights: Array<{ name: string; value: number }>,
  catalog?: { id: string; version: string },
): { body: NamespaceUpsertRequest; dirty: boolean } {
  const body: NamespaceUpsertRequest = {}
  const scalarFields: Array<{
    key: keyof NamespaceUpsertRequest
    initial: number | string
    draft: number | string
  }> = [
    { key: 'lambda', initial: initial.lambda, draft: draft.lambda },
    { key: 'gamma', initial: initial.gamma, draft: draft.gamma },
    { key: 'alpha', initial: initial.alpha, draft: draft.alpha },
    { key: 'max_results', initial: initial.max_results, draft: draft.max_results },
    { key: 'seen_items_days', initial: initial.seen_items_days, draft: draft.seen_items_days },
    { key: 'dense_source', initial: initial.dense_source, draft: draft.dense_source },
    { key: 'embedding_dim', initial: initial.embedding_dim, draft: draft.embedding_dim },
    { key: 'dense_distance', initial: initial.dense_distance, draft: draft.dense_distance },
    { key: 'trending_window', initial: initial.trending_window, draft: draft.trending_window },
    { key: 'trending_ttl', initial: initial.trending_ttl, draft: draft.trending_ttl },
    { key: 'lambda_trending', initial: initial.lambda_trending, draft: draft.lambda_trending },
  ]
  // Booleans stay out of scalarFields, whose diff is typed number | string.
  if ((draft.exclude_authored ?? false) !== (initial.exclude_authored ?? false)) {
    body.exclude_authored = draft.exclude_authored ?? false
  }

  for (const f of scalarFields) {
    if (f.draft !== f.initial) {
      // The pointer-style body accepts numbers and strings as-is — assignment
      // is intentionally `any`-ish to avoid a per-field cast tree.
      ;(body as Record<string, unknown>)[f.key] = f.draft
    }
  }

  // Action weights — rebuild the map from the editor rows, then diff against
  // the initial. We compare entry counts + per-entry values to detect any
  // add / remove / rename / weight change.
  const nextMap: Record<string, number> = {}
  for (const w of weights) {
    const name = w.name.trim()
    if (!name) continue
    if (!Number.isFinite(w.value)) continue
    nextMap[name] = w.value
  }
  const initialWeights = initial.action_weights ?? {}
  const initialKeys = Object.keys(initialWeights)
  const nextKeys = Object.keys(nextMap)
  let weightsChanged = initialKeys.length !== nextKeys.length
  if (!weightsChanged) {
    for (const k of nextKeys) {
      if (initialWeights[k] !== nextMap[k]) {
        weightsChanged = true
        break
      }
    }
  }
  if (weightsChanged) {
    body.action_weights = nextMap
  }

  // dense_source="catalog" is only accepted with its strategy alongside, so the
  // backend can validate the strategy dim against embedding_dim in the same
  // request. Sent whenever the operator is switching into catalog mode, even
  // before both pickers are filled — the submit itself is gated separately, and
  // a partial selection still counts as dirty.
  if (body.dense_source === 'catalog' && catalog) {
    body.catalog_strategy_id = catalog.id
    body.catalog_strategy_version = catalog.version
  }

  return { body, dirty: Object.keys(body).length > 0 }
}

/**
 * excludeAuthoredHelp states plainly whether the toggle can act on anything.
 * The filter reads catalog_items.author_subject_id, so in a namespace with no
 * attributed items it is a silent no-op — which is exactly the case an
 * operator cannot otherwise see from this page.
 */
function excludeAuthoredHelp({
  attributed,
  total,
}: {
  attributed: number
  total: number
}): string {
  const what = 'Drop objects the subject authored from their own recommendations.'
  if (total === 0) {
    return `${what} This namespace has no catalog items, so nothing carries an author and the filter will do nothing.`
  }
  if (attributed === 0) {
    return `${what} None of the ${total} catalog items here carry an author, so the filter will do nothing.`
  }
  return `${what} ${attributed} of ${total} catalog items carry an author.`
}
