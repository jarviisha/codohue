import { useMemo, useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Button,
  Container,
  FormField,
  IconButton,
  Inline,
  Input,
  Select,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  useToast,
} from '@jarviisha/davinci-react-ui'
import {
  useNamespaceDashboard,
  useUpsertNamespace,
  type NamespaceConfig,
  type NamespaceUpsertRequest,
} from '@/services/namespaces'
import PageHeader from '@/components/shell/PageHeader'
import DirtyFormGuard from '@/components/shell/DirtyFormGuard'
import NamespaceTag from '@/components/NamespaceTag'

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
  const toast = useToast()
  const dashboard = useNamespaceDashboard(ns ?? null)
  const upsert = useUpsertNamespace()

  if (!ns) return null

  if (dashboard.isLoading) {
    return (
      <Container size="md" className="py-6 px-6">
        <Skeleton className="h-48 w-full" />
      </Container>
    )
  }

  if (dashboard.isError || !dashboard.data) {
    return (
      <Container size="md" className="py-6 px-6">
        <Alert
          variant="danger"
          title="Could not load namespace config"
          description={dashboard.error?.message ?? 'unknown error'}
        />
      </Container>
    )
  }

  return (
    <ConfigForm
      key={dashboard.data.config.updated_at}
      ns={ns}
      initial={dashboard.data.config}
      onSubmit={(body, onReset) => {
        upsert.mutate(
          { namespace: ns, body },
          {
            onSuccess: () => {
              toast.success('Configuration saved', {
                description: `#${ns} updated.`,
              })
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
  onSubmit,
  saving,
  error,
}: {
  ns: string
  initial: NamespaceConfig
  onSubmit: (body: NamespaceUpsertRequest, onReset: () => void) => void
  saving: boolean
  error: string | undefined
}) {
  const [draft, setDraft] = useState<NamespaceConfig>(initial)
  // Action weights kept as ordered entries so adds/removes don't reshuffle
  // existing rows on every keystroke. A freshly created namespace can come
  // back with action_weights = null, so coalesce before iterating.
  const [weights, setWeights] = useState<Array<{ name: string; value: string }>>(
    () =>
      Object.entries(initial.action_weights ?? {}).map(([name, value]) => ({
        name,
        value: String(value),
      })),
  )

  const dirty = useMemo(() => diffConfig(initial, draft, weights).dirty, [initial, draft, weights])

  const reset = () => {
    setDraft(initial)
    setWeights(
      Object.entries(initial.action_weights ?? {}).map(([name, value]) => ({
        name,
        value: String(value),
      })),
    )
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    const { body } = diffConfig(initial, draft, weights)
    onSubmit(body, reset)
  }

  return (
    <Container size="lg" className="py-6 px-6">
      <DirtyFormGuard dirty={dirty && !saving} />
      <PageHeader>
        <Inline align="center" justify="between" className="w-full" wrap>
          <Stack gap="050">
            <Inline align="center">
              <h1 className="text-foreground text-xl font-semibold">
                Configuration · <NamespaceTag name={ns} />
              </h1>
              {dirty && <Badge variant="warning">unsaved</Badge>}
            </Inline>
            <p className="text-foreground-subtle text-sm">
              Mirrors PUT /api/admin/v1/namespaces/{ns}. Only changed fields are sent.
            </p>
          </Stack>
          <Inline align="center">
            <Button variant="ghost" tone="neutral" disabled={!dirty || saving} onClick={reset}>
              Reset
            </Button>
            <Button onClick={handleSubmit} disabled={!dirty || saving}>
              {saving ? 'Saving…' : 'Save'}
            </Button>
          </Inline>
        </Inline>
      </PageHeader>

      <form onSubmit={handleSubmit}>
        <Stack>
          {error && <Alert variant="danger" title="Save failed" description={error} />}

          <SectionCard
            title="Recommend behavior"
            description="Blend, decay, and seen-items rules applied at serve time."
          >
            <FormField
              label="Alpha (sparse weight)"
              helpText="0 = dense only, 1 = sparse only. Hybrid only kicks in when alpha < 1 AND dense_source ≠ disabled."
            >
              <Input
                type="number"
                step="0.01"
                min={0}
                max={1}
                value={draft.alpha}
                onChange={(e) => setDraft({ ...draft, alpha: Number(e.target.value) })}
              />
            </FormField>
            <FormField
              label="Lambda (time decay)"
              helpText="Higher = older events count less. Applied during sparse vector build."
            >
              <Input
                type="number"
                step="0.001"
                min={0}
                value={draft.lambda}
                onChange={(e) => setDraft({ ...draft, lambda: Number(e.target.value) })}
              />
            </FormField>
            <FormField
              label="Gamma (object freshness)"
              helpText="γ-based freshness rerank at serve time. 0 = disabled."
            >
              <Input
                type="number"
                step="0.001"
                min={0}
                value={draft.gamma}
                onChange={(e) => setDraft({ ...draft, gamma: Number(e.target.value) })}
              />
            </FormField>
            <FormField
              label="Max results"
              helpText="Maximum recommendations the API will return for a single subject."
            >
              <Input
                type="number"
                min={1}
                max={500}
                value={draft.max_results}
                onChange={(e) => setDraft({ ...draft, max_results: Number(e.target.value) })}
              />
            </FormField>
            <FormField
              label="Seen items days"
              helpText="Recency window for the seen-items filter — events older than this are not counted as 'already seen'."
            >
              <Input
                type="number"
                min={1}
                value={draft.seen_items_days}
                onChange={(e) => setDraft({ ...draft, seen_items_days: Number(e.target.value) })}
              />
            </FormField>
          </SectionCard>

          <SectionCard
            title="Dense vectors"
            description="Source + shape of dense embeddings used in the hybrid blend."
          >
            <FormField
              label="Dense source"
              helpText="Single producer of object dense vectors: disabled (sparse only), byoe (you push embeddings), item2vec / svd (cron retrains from events), or catalog (auto-embed ingested content — set the strategy in the Catalog tab)."
            >
              <Select
                value={draft.dense_source}
                onChange={(e) => setDraft({ ...draft, dense_source: e.target.value })}
              >
                {DENSE_SOURCES.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </Select>
            </FormField>
            <FormField
              label="Embedding dim"
              helpText="Width of dense vectors. Must match what your producer sends (BYOE) or what item2vec/svd is configured for."
            >
              <Input
                type="number"
                min={8}
                max={2048}
                value={draft.embedding_dim}
                onChange={(e) => setDraft({ ...draft, embedding_dim: Number(e.target.value) })}
              />
            </FormField>
            <FormField
              label="Dense distance"
              helpText="Similarity metric for the dense Qdrant collections. cosine for normalized embeddings, dot product when magnitude carries signal."
            >
              <Select
                value={draft.dense_distance}
                onChange={(e) => setDraft({ ...draft, dense_distance: e.target.value })}
              >
                {DENSE_DISTANCES.map((d) => (
                  <option key={d.value} value={d.value}>
                    {d.label}
                  </option>
                ))}
              </Select>
            </FormField>
          </SectionCard>

          <SectionCard
            title="Trending"
            description="Cold-start fallback path — Redis ZSET that drives /v1/trending and the hybrid for new subjects."
          >
            <FormField
              label="Trending window (hours)"
              helpText="Events newer than this contribute to the trending score."
            >
              <Input
                type="number"
                min={1}
                value={draft.trending_window}
                onChange={(e) => setDraft({ ...draft, trending_window: Number(e.target.value) })}
              />
            </FormField>
            <FormField
              label="Trending TTL (seconds)"
              helpText="How long the Redis ZSET is cached before recomputation triggers."
            >
              <Input
                type="number"
                min={1}
                value={draft.trending_ttl}
                onChange={(e) => setDraft({ ...draft, trending_ttl: Number(e.target.value) })}
              />
            </FormField>
            <FormField
              label="Lambda trending"
              helpText="Time decay applied while building the trending ZSET. Independent of the sparse lambda."
            >
              <Input
                type="number"
                step="0.001"
                min={0}
                value={draft.lambda_trending}
                onChange={(e) => setDraft({ ...draft, lambda_trending: Number(e.target.value) })}
              />
            </FormField>
          </SectionCard>

          <SectionCard
            title="Action weights"
            description="Per-action weight applied during sparse vector build. Higher = stronger signal."
          >
            <ActionWeightsEditor weights={weights} onChange={setWeights} />
          </SectionCard>
        </Stack>
      </form>
    </Container>
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
    <Stack>
      <Stack className="border-border border-b-2 pb-3">
        <span className="text-foreground text-2xl font-semibold tracking-wide">{title}</span>
        <p className="text-muted text-sm">{description}</p>
      </Stack>
      {children}
    </Stack>
  )
}

function ActionWeightsEditor({
  weights,
  onChange,
}: {
  weights: Array<{ name: string; value: string }>
  onChange: (next: Array<{ name: string; value: string }>) => void
}) {
  const setRow = (idx: number, patch: Partial<{ name: string; value: string }>) => {
    onChange(weights.map((w, i) => (i === idx ? { ...w, ...patch } : w)))
  }
  const removeRow = (idx: number) => onChange(weights.filter((_, i) => i !== idx))
  const addRow = () => onChange([...weights, { name: '', value: '1.0' }])

  if (weights.length === 0) {
    return (
      <Stack>
        <p className="text-foreground-subtle text-sm">No actions configured.</p>
        <Inline justify="end">
          <Button type="button" size="sm" variant="outline" tone="neutral" onClick={addRow}>
            Add action
          </Button>
        </Inline>
      </Stack>
    )
  }

  return (
    <Stack>
      <TableContainer>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Action</TableHead>
              <TableHead align="right">Weight</TableHead>
              <TableHead align="right">Remove</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {weights.map((w, i) => (
              <TableRow key={i}>
                <TableCell>
                  <Input
                    value={w.name}
                    onChange={(e) => setRow(i, { name: e.target.value })}
                    placeholder="e.g. click"
                  />
                </TableCell>
                <TableCell align="right">
                  <Input
                    type="number"
                    step="0.01"
                    value={w.value}
                    onChange={(e) => setRow(i, { value: e.target.value })}
                  />
                </TableCell>
                <TableCell align="right">
                  <IconButton
                    aria-label={`Remove action ${w.name || i}`}
                    variant="ghost"
                    tone="danger"
                    onClick={() => removeRow(i)}
                  >
                    ×
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      <Inline justify="end">
        <Button type="button" size="sm" variant="outline" tone="neutral" onClick={addRow}>
          Add action
        </Button>
      </Inline>
    </Stack>
  )
}

function diffConfig(
  initial: NamespaceConfig,
  draft: NamespaceConfig,
  weights: Array<{ name: string; value: string }>,
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
    const v = Number(w.value)
    if (!Number.isFinite(v)) continue
    nextMap[name] = v
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

  return { body, dirty: Object.keys(body).length > 0 }
}
