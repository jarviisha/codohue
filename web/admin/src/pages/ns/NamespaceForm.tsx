import { useState, type FormEvent, type ReactNode } from 'react'
import {
  Badge,
  Button,
  Field,
  Form,
  FormGrid,
  Input,
  Notice,
  NumberInput,
  RadioGroup,
  Select,
} from '@/components/ui'
import type { RadioOption } from '@/components/ui'
import {
  defaultActionWeights,
  hasErrors,
  normalizeNamespaceName,
  validateNamespaceForm,
  type DenseStrategy,
  type NamespaceFormErrors,
  type NamespaceFormState,
} from './configForm'

type ConfigTab = 'identity' | 'actions' | 'scoring' | 'dense' | 'trending'

interface NamespaceFormProps {
  mode: 'create' | 'edit'
  formId: string
  state: NamespaceFormState
  onChange: (next: NamespaceFormState) => void
  onSubmit: (values: NamespaceFormState) => void
  isPending: boolean
  errorMessage?: string
}

const STRATEGY_OPTIONS: RadioOption<DenseStrategy>[] = [
  {
    value: 'item2vec',
    label: 'item2vec',
    hint: 'Skip-gram trained item embeddings on co-occurrence.',
  },
  {
    value: 'svd',
    label: 'svd',
    hint: 'Truncated SVD over the interaction matrix.',
  },
  {
    value: 'byoe',
    label: 'byoe',
    hint: 'Bring-your-own-embeddings via PUT /objects/:id/embedding.',
  },
  {
    value: 'disabled',
    label: 'disabled',
    hint: 'Skip the dense phase entirely (sparse-only recommendations).',
  },
]

const TAB_LABEL: Record<ConfigTab, string> = {
  identity: 'identity',
  actions: 'actions',
  scoring: 'scoring',
  dense: 'dense',
  trending: 'trending',
}

function firstErrorTab(errors: NamespaceFormErrors, mode: 'create' | 'edit') {
  if (mode === 'create' && errors.namespace) return 'identity'
  if (errors.action_weights) return 'actions'
  if (
    errors.lambda ||
    errors.gamma ||
    errors.alpha ||
    errors.max_results ||
    errors.seen_items_days
  ) {
    return 'scoring'
  }
  if (errors.embedding_dim) return 'dense'
  if (
    errors.trending_window ||
    errors.trending_ttl ||
    errors.lambda_trending
  ) {
    return 'trending'
  }
  return null
}

function tabHasError(tab: ConfigTab, errors: NamespaceFormErrors) {
  switch (tab) {
    case 'identity':
      return Boolean(errors.namespace)
    case 'actions':
      return Boolean(errors.action_weights)
    case 'scoring':
      return Boolean(
        errors.lambda ||
          errors.gamma ||
          errors.alpha ||
          errors.max_results ||
          errors.seen_items_days,
      )
    case 'dense':
      return Boolean(errors.embedding_dim)
    case 'trending':
      return Boolean(
        errors.trending_window ||
          errors.trending_ttl ||
          errors.lambda_trending,
      )
  }
}

function TabPanel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="bg-surface border border-default rounded-sm">
      <div className="px-5 py-4 border-b border-default">
        <h2 className="text-sm font-semibold text-primary">{title}</h2>
      </div>
      <div className="px-5 py-5">{children}</div>
    </section>
  )
}

export default function NamespaceForm({
  mode,
  formId,
  state,
  onChange,
  onSubmit,
  isPending,
  errorMessage,
}: NamespaceFormProps) {
  const tabs: ConfigTab[] =
    mode === 'create'
      ? ['identity', 'actions', 'scoring', 'dense', 'trending']
      : ['actions', 'scoring', 'dense', 'trending']
  const [activeTab, setActiveTab] = useState<ConfigTab>(tabs[0])
  const [errors, setErrors] = useState<NamespaceFormErrors>(() => ({}))
  const [submitted, setSubmitted] = useState(false)

  const propagate = (next: NamespaceFormState) => {
    onChange(next)
    if (submitted) setErrors(validateNamespaceForm(next, mode))
  }

  const update = <K extends keyof NamespaceFormState>(
    key: K,
    value: NamespaceFormState[K],
  ) => {
    propagate({ ...state, [key]: value })
  }

  const updateNumber = <K extends keyof NamespaceFormState>(
    key: K,
    raw: string,
  ) => {
    if (raw.trim() === '') return
    const next = Number(raw)
    if (!Number.isFinite(next)) return
    update(key, next as NamespaceFormState[K])
  }

  const updateAction = (
    i: number,
    field: keyof NamespaceFormState['action_weights'][number],
    value: string | number,
  ) => {
    const rows = [...state.action_weights]
    rows[i] = { ...rows[i], [field]: value }
    propagate({ ...state, action_weights: rows })
  }

  const updateActionWeight = (i: number, raw: string) => {
    if (raw.trim() === '') return
    const next = Number(raw)
    if (!Number.isFinite(next)) return
    updateAction(i, 'weight', next)
  }

  const addAction = () => {
    propagate({
      ...state,
      action_weights: [...state.action_weights, { action: '', weight: 0 }],
    })
  }

  const useDefaultActions = () => {
    propagate({
      ...state,
      action_weights: defaultActionWeights.map((row) => ({ ...row })),
    })
  }

  const removeAction = (i: number) => {
    propagate({
      ...state,
      action_weights: state.action_weights.filter((_, idx) => idx !== i),
    })
  }

  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (isPending) return
    setSubmitted(true)
    const nextState =
      mode === 'create'
        ? { ...state, namespace: normalizeNamespaceName(state.namespace) }
        : state
    if (nextState !== state) onChange(nextState)
    const nextErrors = validateNamespaceForm(nextState, mode)
    setErrors(nextErrors)
    const nextTab = firstErrorTab(nextErrors, mode)
    if (nextTab) setActiveTab(nextTab)
    if (hasErrors(nextErrors)) return
    onSubmit(nextState)
  }

  return (
    <Form id={formId} onSubmit={submit}>
      {errorMessage ? (
        <Notice tone="fail" title="Save failed">
          {errorMessage}
        </Notice>
      ) : null}

      <div className="grid grid-cols-1 lg:grid-cols-[12rem_minmax(0,1fr)] gap-5 items-start">
        <div
          role="tablist"
          aria-label="Namespace config sections"
          aria-orientation="vertical"
          className="flex flex-row lg:flex-col gap-1 overflow-x-auto lg:overflow-visible border-b lg:border-b-0 border-default pb-2 lg:pb-0"
        >
          {tabs.map((tab) => {
            const selected = tab === activeTab
            const invalid = tabHasError(tab, errors)
            return (
              <button
                key={tab}
                type="button"
                role="tab"
                aria-selected={selected}
                aria-controls={`ns-config-${tab}`}
                onClick={() => setActiveTab(tab)}
                className={[
                  'h-9 px-3 rounded-sm font-mono text-xs uppercase tracking-[0.04em] border border-transparent text-left shrink-0',
                  selected
                    ? 'bg-accent-subtle text-accent border-default'
                    : 'text-secondary hover:bg-surface-raised hover:text-primary',
                  invalid ? 'text-danger' : '',
                ].join(' ')}
              >
                {TAB_LABEL[tab]}
              </button>
            )
          })}
        </div>

        <div id={`ns-config-${activeTab}`} role="tabpanel" className="min-w-0">
        {activeTab === 'identity' ? (
          <TabPanel title="Identity">
            <Field
              label="Namespace name"
              htmlFor="ns-name"
              required
              error={errors.namespace}
              hint="Lowercase letters, digits, underscore, dash. This is permanent."
            >
              <Input
                id="ns-name"
                value={state.namespace}
                onChange={(e) => update('namespace', e.target.value)}
                onBlur={(e) =>
                  update('namespace', normalizeNamespaceName(e.target.value))
                }
                placeholder="prod"
                invalid={Boolean(errors.namespace)}
                autoFocus
              />
            </Field>
          </TabPanel>
        ) : null}

        {activeTab === 'actions' ? (
          <TabPanel title="Action weights">
            <div className="flex flex-wrap items-center justify-between gap-3 mb-4">
              <div className="flex items-center gap-2">
                <Badge>{state.action_weights.length}</Badge>
                <span className="text-sm text-secondary">
                  configured action weights
                </span>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  size="xs"
                  onClick={useDefaultActions}
                >
                  reset default
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  size="xs"
                  onClick={addAction}
                >
                  add action
                </Button>
              </div>
            </div>
            {errors.action_weights ? (
              <div className="mb-3">
                <Notice tone="fail">{errors.action_weights}</Notice>
              </div>
            ) : null}
            <div className="hidden sm:grid grid-cols-[minmax(0,1fr)_8rem_5.5rem] gap-2 px-1 pb-2 border-b border-default">
              <div className="font-mono text-xs uppercase tracking-[0.04em] text-secondary">
                action
              </div>
              <div className="font-mono text-xs uppercase tracking-[0.04em] text-secondary text-right">
                weight
              </div>
              <div className="font-mono text-xs uppercase tracking-[0.04em] text-secondary text-right">
                row
              </div>
            </div>
            <div className="flex flex-col gap-2 mt-2">
              {state.action_weights.map((row, i) => (
                <div
                  key={i}
                  className="grid grid-cols-1 sm:grid-cols-[minmax(0,1fr)_8rem_5.5rem] gap-2 items-center"
                >
                  <Input
                    inputSize="sm"
                    value={row.action}
                    onChange={(e) => updateAction(i, 'action', e.target.value)}
                    placeholder="action name"
                    className="w-full"
                    aria-label={`action name row ${i + 1}`}
                  />
                  <NumberInput
                    width="w-full"
                    value={row.weight}
                    onChange={(e) => updateActionWeight(i, e.target.value)}
                    step={0.1}
                    aria-label={`weight row ${i + 1}`}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="sm:justify-self-end"
                    onClick={() => removeAction(i)}
                  >
                    remove
                  </Button>
                </div>
              ))}
            </div>
          </TabPanel>
        ) : null}

        {activeTab === 'scoring' ? (
          <TabPanel title="Decay + scoring">
            <FormGrid columns={2}>
              <Field
                label="lambda (event decay rate)"
                htmlFor="ns-lambda"
                error={errors.lambda}
                hint="e^(-lambda x days_since)"
              >
                <NumberInput
                  id="ns-lambda"
                  width="w-36"
                  value={state.lambda}
                  onChange={(e) => updateNumber('lambda', e.target.value)}
                  step={0.01}
                  min={0}
                  invalid={Boolean(errors.lambda)}
                />
              </Field>
              <Field
                label="gamma (object freshness)"
                htmlFor="ns-gamma"
                error={errors.gamma}
                hint="Applied at rerank time."
              >
                <NumberInput
                  id="ns-gamma"
                  width="w-36"
                  value={state.gamma}
                  onChange={(e) => updateNumber('gamma', e.target.value)}
                  step={0.01}
                  min={0}
                  invalid={Boolean(errors.gamma)}
                />
              </Field>
              <Field
                label="alpha (sparse vs dense)"
                htmlFor="ns-alpha"
                error={errors.alpha}
                hint="0.0 = pure dense, 1.0 = pure sparse"
              >
                <NumberInput
                  id="ns-alpha"
                  width="w-36"
                  value={state.alpha}
                  onChange={(e) => updateNumber('alpha', e.target.value)}
                  step={0.05}
                  min={0}
                  max={1}
                  invalid={Boolean(errors.alpha)}
                />
              </Field>
              <Field
                label="max results"
                htmlFor="ns-maxr"
                error={errors.max_results}
              >
                <NumberInput
                  id="ns-maxr"
                  width="w-36"
                  value={state.max_results}
                  onChange={(e) =>
                    updateNumber('max_results', e.target.value)
                  }
                  step={1}
                  min={1}
                  invalid={Boolean(errors.max_results)}
                />
              </Field>
              <Field
                label="seen items days"
                htmlFor="ns-seen"
                hint="Recency window for the seen-items filter."
                error={errors.seen_items_days}
              >
                <NumberInput
                  id="ns-seen"
                  width="w-36"
                  value={state.seen_items_days}
                  onChange={(e) =>
                    updateNumber('seen_items_days', e.target.value)
                  }
                  step={1}
                  min={0}
                  invalid={Boolean(errors.seen_items_days)}
                />
              </Field>
            </FormGrid>
          </TabPanel>
        ) : null}

        {activeTab === 'dense' ? (
          <TabPanel title="Dense strategy">
            <Field label="Strategy" htmlFor="ns-strategy">
              <RadioGroup<DenseStrategy>
                name="ns-strategy"
                value={state.dense_strategy}
                onChange={(v) => update('dense_strategy', v)}
                options={STRATEGY_OPTIONS}
              />
            </Field>
            <div className="mt-4">
              <FormGrid columns={2}>
                <Field
                  label="embedding dim"
                  htmlFor="ns-dim"
                  error={errors.embedding_dim}
                  hint="Must match the strategy output (or your BYOE vectors)."
                >
                  <NumberInput
                    id="ns-dim"
                    width="w-36"
                    value={state.embedding_dim}
                    onChange={(e) =>
                      updateNumber('embedding_dim', e.target.value)
                    }
                    step={1}
                    min={1}
                    invalid={Boolean(errors.embedding_dim)}
                  />
                </Field>
                <Field label="distance" htmlFor="ns-distance">
                  <Select
                    id="ns-distance"
                    value={state.dense_distance}
                    onChange={(e) => update('dense_distance', e.target.value)}
                  >
                    <option value="cosine">cosine</option>
                    <option value="dot">dot</option>
                    <option value="euclidean">euclidean</option>
                  </Select>
                </Field>
              </FormGrid>
            </div>
          </TabPanel>
        ) : null}

        {activeTab === 'trending' ? (
          <TabPanel title="Trending">
            <FormGrid columns={2}>
              <Field
                label="window (hours)"
                htmlFor="ns-tr-win"
                hint="Events older than this don't contribute."
                error={errors.trending_window}
              >
                <NumberInput
                  id="ns-tr-win"
                  width="w-36"
                  value={state.trending_window}
                  onChange={(e) =>
                    updateNumber('trending_window', e.target.value)
                  }
                  step={1}
                  min={1}
                  invalid={Boolean(errors.trending_window)}
                />
              </Field>
              <Field
                label="TTL (seconds)"
                htmlFor="ns-tr-ttl"
                hint="Redis ZSET expiry for cached trending scores."
                error={errors.trending_ttl}
              >
                <NumberInput
                  id="ns-tr-ttl"
                  width="w-36"
                  value={state.trending_ttl}
                  onChange={(e) =>
                    updateNumber('trending_ttl', e.target.value)
                  }
                  step={1}
                  min={1}
                  invalid={Boolean(errors.trending_ttl)}
                />
              </Field>
              <Field
                label="lambda trending"
                htmlFor="ns-tr-lambda"
                error={errors.lambda_trending}
                hint="Time-decay rate for the trending score."
              >
                <NumberInput
                  id="ns-tr-lambda"
                  width="w-36"
                  value={state.lambda_trending}
                  onChange={(e) =>
                    updateNumber('lambda_trending', e.target.value)
                  }
                  step={0.01}
                  min={0}
                  invalid={Boolean(errors.lambda_trending)}
                />
              </Field>
            </FormGrid>
          </TabPanel>
        ) : null}
        </div>
      </div>
    </Form>
  )
}
