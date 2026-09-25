import { useRef, useState } from 'react'
import { useLocation, useParams, useSearchParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import {
  AlertDialog,
  Banner,
  Button,
  Heading,
  Layout,
  Link,
  Selector,
  Skeleton,
  Stack,
  Switch,
  Tab,
  TabList,
  TextArea,
  TextInput,
} from '@astryxdesign/core'
import PageHeader from '@/components/shell/PageHeader'
import DirtyFormGuard from '@/components/shell/DirtyFormGuard'
import CatalogStrategyFields from '@/components/CatalogStrategyFields'
import { useCatalogStrategies } from '@/services/catalog'
import { ApiError, apiFetch } from '@/services/http'
import {
  configurationPath,
  draftChanges,
  groupNames,
  parseChanges,
  toDraft,
  useConfiguration,
  type Configuration,
  type Draft,
  type GroupName,
} from '@/services/configuration'
import { useRuntime } from '@/services/runtime'

const labels: Record<GroupName, string> = {
  recommendations: 'Recommendations',
  signals: 'Event signals',
  trending: 'Trending',
  embeddings: 'Embeddings',
}
const descriptions: Record<GroupName, string> = {
  recommendations: 'Tune ranking and decide which items a subject can receive.',
  signals: 'Define the strength and decay of behavioral events.',
  trending: 'Control the event window and lifetime of popular-item rankings.',
  embeddings: 'Choose the producer and shape of dense vectors, with catalog settings in one place.',
}
const fields: Record<string, [string, string]> = {
  alpha: [
    'Alpha (sparse weight)',
    '0 = dense only; 1 = sparse only. Hybrid requires an active dense source.',
  ],
  gamma: ['Gamma (object freshness)', 'Freshness reranking at serve time. 0 disables it.'],
  max_results: ['Max results', 'Maximum recommendations returned for a subject.'],
  seen_items_days: [
    'Seen items window (days)',
    'How long interactions count toward the seen-items filter.',
  ],
  lambda: [
    'Lambda (time decay)',
    'Higher values reduce the contribution of older events to sparse vectors.',
  ],
  trending_window: [
    'Trending window (hours)',
    'Events inside this window contribute to the next batch.',
  ],
  trending_ttl: [
    'Trending TTL (seconds)',
    'Cache lifetime after a rebuild. Allow time for the batch interval and execution.',
  ],
  lambda_trending: [
    'Trending decay',
    'Higher values favor recent events when the next ranking is built.',
  ],
  embedding_dim: ['Embedding dimension', 'Must match the output dimension of your producer.'],
  catalog_max_attempts: [
    'Embedding retry limit',
    'The embedder process supplies the inherited default.',
  ],
  catalog_max_content_bytes: [
    'Content limit (bytes)',
    'The API process supplies the inherited default.',
  ],
}
type LocalGroup = {
  generation: number
  revision: number
  baseline: Draft
  draft: Draft
  errors: Record<string, string>
  failure?: string
  conflict?: Configuration
  notice?: string
}
type Edits = Partial<Record<GroupName, LocalGroup>>
function fresh(config: Configuration, group: GroupName): LocalGroup {
  const baseline = toDraft(config.groups[group].values)
  return {
    generation: config.generation,
    revision: config.groups[group].revision,
    baseline,
    draft: { ...baseline },
    errors: {},
  }
}
export default function NamespaceConfigPage() {
  const { ns } = useParams<{ ns: string }>()
  return ns ? <Settings key={ns} ns={ns} /> : null
}
function Settings({ ns }: { ns: string }) {
  const query = useConfiguration(ns),
    client = useQueryClient()
  const [search, setSearch] = useSearchParams(),
    location = useLocation()
  const anchors: Record<string, GroupName> = {
    '#catalog-settings': 'embeddings',
    '#embeddings-catalog': 'embeddings',
    '#event-signals': 'signals',
    '#trending': 'trending',
  }
  const selected = search.get('tab') ?? anchors[location.hash] ?? 'recommendations'
  const group: GroupName = groupNames.includes(selected as GroupName)
    ? (selected as GroupName)
    : 'recommendations'
  const [edits, setEdits] = useState<Edits>({})
  const [saving, setSaving] = useState<GroupName | null>(null)
  const [review, setReview] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const errorRef = useRef<HTMLDivElement>(null)
  const data = query.data
  if (!data)
    return (
      <Layout height="auto" contentWidth={1152}>
        {query.isLoading ? (
          <Skeleton height={192} />
        ) : (
          <Banner
            status="error"
            title="Could not load settings"
            description={query.error?.message}
          />
        )}
      </Layout>
    )
  const local = edits[group]
  const current =
    local &&
    (draftChanges(local.baseline, local.draft).length > 0 ||
      (local.generation === data.generation && local.revision === data.groups[group].revision))
      ? local
      : fresh(data, group)
  const changed = draftChanges(current.baseline, current.draft)
  const dirtyGroups = groupNames.filter(
    (g) => edits[g] && draftChanges(edits[g]!.baseline, edits[g]!.draft).length > 0,
  )
  const update = (patch: Draft) =>
    setEdits((prev) => {
      const state = current
      return {
        ...prev,
        [group]: { ...state, draft: { ...state.draft, ...patch }, notice: undefined },
      }
    })
  const discard = () => setEdits((prev) => ({ ...prev, [group]: undefined }))
  const showErrors = (
    errors: Record<string, string>,
    failure?: string,
    conflict?: Configuration,
  ) => {
    setEdits((prev) => ({
      ...prev,
      [group]: { ...(prev[group] ?? current), errors, failure, conflict },
    }))
    requestAnimationFrame(() => errorRef.current?.focus())
  }
  const save = async () => {
    const parsed = parseChanges(current.baseline, current.draft)
    if (Object.keys(parsed.errors).length) {
      showErrors(parsed.errors)
      return
    }
    setSaving(group)
    try {
      const saved = await apiFetch<Configuration>(configurationPath(ns), {
        method: 'PATCH',
        body: JSON.stringify({
          group,
          generation: current.generation,
          base_revision: current.revision,
          changes: parsed.changes,
        }),
      })
      client.setQueryData(['configuration', ns], saved)
      setEdits((prev) => ({
        ...prev,
        [group]: {
          ...fresh(saved, group),
          notice: `${labels[group]} saved. ${saved.groups[group].guidance}`,
        },
      }))
      void client.invalidateQueries({ queryKey: ['namespaces'] })
      void client.invalidateQueries({ queryKey: ['ns', ns] })
      void client.invalidateQueries({ queryKey: ['overview'] })
      void client.invalidateQueries({ queryKey: ['catalog'] })
    } catch (error) {
      const e = error as ApiError
      const errors = Object.fromEntries(
        Object.entries(e.details?.fields ?? {}).map(([k, v]) => [
          k.split('.').slice(1).join('.'),
          v,
        ]),
      )
      showErrors(errors, e.message, e.details?.current as Configuration | undefined)
    } finally {
      setSaving(null)
    }
  }
  const reconcile = () => {
    const server = current.conflict!
    const next = fresh(server, group)
    // Carry forward only the user's changed fields, retaining concurrent edits elsewhere.
    for (const field of changed) next.draft[field] = current.draft[field]
    client.setQueryData(['configuration', ns], server)
    setEdits((prev) => ({ ...prev, [group]: next }))
  }
  const hasErrors = Object.keys(current.errors).length > 0 || !!current.failure
  return (
    <Layout height="auto" contentWidth={1152}>
      <PageHeader>
        <Stack gap={1}>
          <Heading level={1}>Settings · {ns}</Heading>
          <p className="text-secondary text-sm">Save each section independently.</p>
        </Stack>
      </PageHeader>
      <DirtyFormGuard dirty={dirtyGroups.length > 0} />
      <AlertDialog
        isOpen={confirm}
        onOpenChange={setConfirm}
        title="Change embedding producer?"
        description="New writes will use the selected producer. Existing vectors are retained and may need rebuilding. This save does not run a job."
        actionLabel="Save embeddings"
        cancelLabel="Keep editing"
        onAction={() => {
          setConfirm(false)
          void save()
        }}
      />
      <Stack gap={6}>
        <TabList
          value={group}
          onChange={(value) => {
            setSearch({ tab: value })
            setReview(false)
          }}
          role="tablist"
          hasDivider
        >
          {groupNames.map((g) => (
            <Tab
              key={g}
              value={g}
              panelId={`settings-${g}`}
              label={`${labels[g]}${edits[g]?.failure || Object.keys(edits[g]?.errors ?? {}).length ? ' · Needs attention' : dirtyGroups.includes(g) ? ' · Unsaved' : ''}`}
            />
          ))}
        </TabList>
        {query.error && (
          <Banner
            status="warning"
            title="Could not refresh settings"
            description="Your drafts are retained. Saving will check for concurrent changes."
          />
        )}
        <Stack
          id={`settings-${group}`}
          role="tabpanel"
          aria-label={labels[group]}
          gap={6}
          tabIndex={0}
        >
          <Stack gap={1}>
            <Heading level={2}>{labels[group]}</Heading>
            <p className="text-secondary">{descriptions[group]}</p>
          </Stack>
          {current.notice && (
            <p role="status" className="text-secondary">
              {current.notice}
            </p>
          )}
          {hasErrors && (
            <Stack ref={errorRef} tabIndex={-1} role="alert" gap={2}>
              <Heading level={3}>
                {current.conflict ? 'Settings changed elsewhere' : 'Could not save this section'}
              </Heading>
              {current.failure && <p>{current.failure}</p>}
              {Object.entries(current.errors).map(([field, message]) =>
                // A cross-group failure is reported against "values", which owns
                // no control; render it as text rather than a link to nowhere.
                field === 'values' ? (
                  <p key={field}>{message}</p>
                ) : (
                  <Link
                    key={field}
                    href={`#field-${field}`}
                    onClick={(e) => {
                      e.preventDefault()
                      document.getElementById(`field-${field}`)?.focus()
                    }}
                  >
                    {fields[field]?.[0] ?? field}: {message}
                  </Link>
                ),
              )}
            </Stack>
          )}
          {current.conflict && (
            <Stack gap={3} className="border border-border rounded-lg p-4">
              <Heading level={3}>Review concurrent changes</Heading>
              {Object.keys(current.draft)
                .filter(
                  (f) =>
                    changed.includes(f) ||
                    JSON.stringify(current.conflict!.groups[group].values[f]) !==
                      JSON.stringify(data.groups[group].values[f]),
                )
                .map((f) => (
                  <Stack gap={1} key={f} className="break-all">
                    <strong>{fields[f]?.[0] ?? f}</strong>
                    <p className="text-sm">Before: {String(current.baseline[f])}</p>
                    <p className="text-sm">
                      Current server: {JSON.stringify(current.conflict!.groups[group].values[f])}
                    </p>
                    <p className="text-sm">Your draft: {String(current.draft[f])}</p>
                  </Stack>
                ))}
              {current.conflict.generation === current.generation ? (
                <Button
                  variant="secondary"
                  label="Keep my changed fields for review"
                  onClick={reconcile}
                />
              ) : (
                <Banner
                  status="warning"
                  title="Namespace was recreated"
                  description="Discard this section's old draft and reload its new settings."
                />
              )}
              <Button
                variant="ghost"
                label="Use server values"
                onClick={() => {
                  client.setQueryData(['configuration', ns], current.conflict)
                  discard()
                }}
              />
            </Stack>
          )}
          <Stack className="grid grid-cols-1 lg:grid-cols-3" gap={8}>
            <form
              className="min-w-0 lg:col-span-2 pb-6"
              onSubmit={(e) => {
                e.preventDefault()
                if (group === 'embeddings' && changed.includes('dense_source')) setConfirm(true)
                else void save()
              }}
              id="namespace-settings-form"
            >
              <fieldset disabled={saving !== null} className="min-w-0 border-0 m-0 p-0">
                <SettingsFields
                  key={`${group}-${current.generation}-${current.revision}`}
                  group={group}
                  draft={current.draft}
                  update={update}
                  errors={current.errors}
                  locks={data.groups[group].locks}
                  defaults={data.defaults}
                />
              </fieldset>
            </form>
            <Stack
              gap={4}
              className="min-w-0 border-t lg:border-t-0 lg:border-l border-border pt-4 lg:pt-0 lg:pl-6"
            >
              <Heading level={3}>When changes take effect</Heading>
              <p className="text-secondary">{data.groups[group].guidance}</p>
              <p className="text-secondary text-sm">
                Only {labels[group].toLowerCase()} is saved. Drafts in other tabs stay available.
              </p>
              {(group === 'signals' || group === 'trending') && (
                <Link href={`/ns/${encodeURIComponent(ns)}/batch-runs`}>Open batch runs</Link>
              )}
              {group === 'embeddings' && (
                <Link href={`/ns/${encodeURIComponent(ns)}/catalog`}>Open catalog operations</Link>
              )}
              <Link href="/system/runtime">View process defaults</Link>
            </Stack>
          </Stack>
          {review && changed.length > 0 && (
            <Stack gap={3}>
              <Heading level={3}>Review {labels[group].toLowerCase()}</Heading>
              {changed.map((f) => (
                <Stack gap={1} key={f} className="border-b border-border pb-3 break-all">
                  <strong>{fields[f]?.[0] ?? f.replaceAll('_', ' ')}</strong>
                  <p className="text-secondary text-sm">
                    {String(current.baseline[f] ?? 'System default')} →{' '}
                    {String(current.draft[f] ?? 'System default')}
                  </p>
                </Stack>
              ))}
            </Stack>
          )}
        </Stack>
        {changed.length > 0 && (
          <Stack
            direction="horizontal"
            align="center"
            justify="between"
            gap={3}
            className="sticky bottom-0 z-10 bg-surface border-t border-border py-4 flex-wrap"
          >
            <p role="status" className="text-sm">
              {changed.length} changed · {labels[group]}
            </p>
            <Stack direction="horizontal" gap={2} className="flex-wrap">
              <Button variant="ghost" label="Review changes" onClick={() => setReview(!review)} />
              <Button
                variant="secondary"
                label="Discard"
                isDisabled={saving !== null}
                onClick={discard}
              />
              <Button
                label={`Save ${labels[group].toLowerCase()}`}
                type="submit"
                form="namespace-settings-form"
                isLoading={saving === group}
                isDisabled={saving !== null || !!current.conflict}
              />
            </Stack>
          </Stack>
        )}
      </Stack>
    </Layout>
  )
}

function SettingsFields({
  group,
  draft,
  update,
  errors,
  locks,
  defaults,
}: {
  group: GroupName
  draft: Draft
  update: (patch: Draft) => void
  errors: Record<string, string>
  locks: Record<string, string>
  defaults?: Configuration['defaults']
}) {
  const dim = Number(draft.embedding_dim),
    catalog = draft.dense_source === 'catalog'
  const strategies = useCatalogStrategies(
    dim,
    group === 'embeddings' && catalog && Number.isSafeInteger(dim) && dim > 0,
  )
  const runtime = useRuntime()
  const input = (field: string) => (
    <TextInput
      key={field}
      id={`field-${field}`}
      label={fields[field][0]}
      description={locks[field] ?? fields[field][1]}
      value={String(draft[field] ?? '')}
      onChange={(v) => update({ [field]: v })}
      isDisabled={!!locks[field]}
      status={errors[field] ? { type: 'error', message: errors[field] } : undefined}
    />
  )
  return (
    <Stack gap={6}>
      {group === 'recommendations' && (
        <>
          {['alpha', 'gamma', 'max_results', 'seen_items_days'].map(input)}
          <Stack gap={3} id="field-exclude_authored" tabIndex={-1}>
            <Switch
              label="Exclude self-authored items"
              description="Exclude items whose author matches the requesting subject. Requires author attribution on catalog items."
              value={!!draft.exclude_authored}
              onChange={(v) => update({ exclude_authored: v })}
            />
          </Stack>
        </>
      )}
      {group === 'signals' && (
        <>
          {input('lambda')}
          <WeightFields
            value={String(draft.action_weights ?? '{}')}
            onChange={(v) => update({ action_weights: v })}
            error={errors.action_weights}
          />
        </>
      )}
      {group === 'trending' && (
        <>
          {['trending_window', 'trending_ttl', 'lambda_trending'].map(input)}
          {runtime.data?.processes.some(
            (p) =>
              p.process === 'cron' &&
              p.settings.some(
                (s) =>
                  s.name === 'batch_interval_minutes' &&
                  Number(draft.trending_ttl) <= Number(s.value) * 60,
              ),
          ) && (
            <Banner
              status="warning"
              title="Cache may expire between batches"
              description="Increase the TTL to allow for the cron interval and batch execution time."
            />
          )}
        </>
      )}
      {group === 'embeddings' && (
        <>
          <Stack gap={3} id="field-dense_source" tabIndex={-1}>
            <Selector
              label="Dense source"
              value={String(draft.dense_source)}
              onChange={(v) => update({ dense_source: v })}
              options={[
                { value: 'disabled', label: 'Disabled — sparse only' },
                { value: 'item2vec', label: 'Item2vec — trained from events' },
                { value: 'svd', label: 'SVD — matrix factorisation' },
                { value: 'byoe', label: 'BYOE — external embeddings' },
                { value: 'catalog', label: 'Catalog — automatic embeddings' },
              ]}
            />
          </Stack>
          {draft.dense_source !== 'disabled' && (
            <>
              {input('embedding_dim')}
              <Stack gap={3} id="field-dense_distance" tabIndex={-1}>
                <Selector
                  label="Dense distance"
                  description={locks.dense_distance ?? 'Similarity metric for dense vectors.'}
                  isDisabled={!!locks.dense_distance}
                  value={String(draft.dense_distance)}
                  onChange={(v) => update({ dense_distance: v })}
                  options={[
                    { value: 'cosine', label: 'Cosine' },
                    { value: 'dot', label: 'Dot product' },
                  ]}
                />
              </Stack>
            </>
          )}
          {catalog && (
            <>
              <Stack gap={3} id="field-catalog_strategy_id" tabIndex={-1}>
                <CatalogStrategyFields
                  embeddingDim={dim}
                  descriptors={strategies.data?.strategies ?? []}
                  loading={strategies.isLoading}
                  error={strategies.error?.message}
                  strategyId={String(draft.catalog_strategy_id ?? '')}
                  strategyVersion={String(draft.catalog_strategy_version ?? '')}
                  onStrategyId={(v) =>
                    update({
                      catalog_strategy_id: v,
                      catalog_strategy_version:
                        strategies.data?.strategies.find((s) => s.id === v)?.version ?? '',
                    })
                  }
                  onStrategyVersion={(v) => update({ catalog_strategy_version: v })}
                />
                {errors.catalog_strategy_id && <p role="alert">{errors.catalog_strategy_id}</p>}
              </Stack>
              <TextArea
                id="field-catalog_strategy_params"
                label="Strategy parameters (JSON)"
                value={String(draft.catalog_strategy_params ?? '{}')}
                onChange={(v) => update({ catalog_strategy_params: v })}
                status={
                  errors.catalog_strategy_params
                    ? { type: 'error', message: errors.catalog_strategy_params }
                    : undefined
                }
              />
              {['catalog_max_attempts', 'catalog_max_content_bytes'].map((field) => {
                const observation = defaults?.[field]
                const process =
                  observation?.process ?? (field === 'catalog_max_attempts' ? 'embedder' : 'api')
                const known = observation?.state === 'observed'
                const reports = observation?.reports ?? []
                const value = observation?.value
                return (
                  <Stack key={field} gap={3}>
                    <Switch
                      label={`Use system default: ${fields[field][0].toLowerCase()}`}
                      value={draft[field] === null}
                      onChange={(v) => update({ [field]: v ? null : known ? String(value) : '' })}
                      description={
                        known
                          ? `Reported by ${process}: ${value} (${reports.length} instance${reports.length === 1 ? '' : 's'}).`
                          : `${process} default ${observation?.state === 'mixed' ? 'differs across instances' : 'has no recent authoritative report'}. Inheritance still applies.`
                      }
                    />
                    {draft[field] !== null && input(field)}
                  </Stack>
                )
              })}
            </>
          )}
        </>
      )}
    </Stack>
  )
}

type WeightRow = { id: string; name: string; value: string }
function WeightFields({
  value,
  onChange,
  error,
}: {
  value: string
  onChange: (v: string) => void
  error?: string
}) {
  const decode = (raw: string): WeightRow[] =>
    raw.startsWith('Invalid action weights: ')
      ? (JSON.parse(raw.slice(24)) as WeightRow[])
      : Object.entries(JSON.parse(raw) as Record<string, number>).map(([name, v]) => ({
          id: crypto.randomUUID(),
          name,
          value: String(v),
        }))
  const [last, setLast] = useState(value)
  const [rows, setRows] = useState<WeightRow[]>(() => decode(value))
  if (last !== value) {
    setLast(value)
    setRows(decode(value))
  }
  const change = (next: WeightRow[]) => {
    setRows(next)
    const names = next.map((r) => r.name.trim())
    const valid =
      names.every(Boolean) &&
      new Set(names).size === names.length &&
      next.every((r) => r.value.trim() && Number.isFinite(Number(r.value)))
    // Invalid rows remain a dirty draft, and cannot be silently dropped on save.
    const encoded = valid
      ? JSON.stringify(
          Object.fromEntries(next.map((r) => [r.name.trim(), Number(r.value)])),
          null,
          2,
        )
      : 'Invalid action weights: ' + JSON.stringify(next)
    setLast(encoded)
    onChange(encoded)
  }
  return (
    <Stack gap={4} id="field-action_weights" tabIndex={-1}>
      <Stack gap={1}>
        <Heading level={3}>Action weights</Heading>
        <p className="text-secondary text-sm">
          Positive weights strengthen signals; negative weights reduce them. Each action name must
          be unique.
        </p>
      </Stack>
      {error && <p role="alert">{error}</p>}
      {rows.map((row, index) => (
        <Stack key={row.id} direction="horizontal" gap={3} align="end" className="min-w-0">
          <TextInput
            label={`Action name, row ${index + 1}`}
            value={row.name}
            status={
              !row.name.trim()
                ? { type: 'error', message: 'Enter an action name.' }
                : rows.filter((r) => r.name.trim() === row.name.trim()).length > 1
                  ? { type: 'error', message: 'Action names must be unique.' }
                  : undefined
            }
            onChange={(v) => change(rows.map((r) => (r.id === row.id ? { ...r, name: v } : r)))}
          />
          <TextInput
            label={`Weight for ${row.name || `row ${index + 1}`}`}
            value={row.value}
            status={
              !row.value.trim() || !Number.isFinite(Number(row.value))
                ? { type: 'error', message: 'Enter a finite number.' }
                : undefined
            }
            onChange={(v) => change(rows.map((r) => (r.id === row.id ? { ...r, value: v } : r)))}
          />
          <Button
            label="Remove"
            aria-label={`Remove action ${row.name || index + 1}`}
            variant="ghost"
            onClick={() => change(rows.filter((r) => r.id !== row.id))}
          />
        </Stack>
      ))}
      <Button
        variant="secondary"
        label="Add action"
        onClick={() => change([...rows, { id: crypto.randomUUID(), name: '', value: '1' }])}
      />
    </Stack>
  )
}
