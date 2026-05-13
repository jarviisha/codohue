import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
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
  Section,
  Select,
} from '../../components/ui'
import type { RadioOption } from '../../components/ui'
import {
  hasErrors,
  validateNamespaceForm,
  type DenseStrategy,
  type NamespaceFormState,
} from './configForm'

interface NamespaceFormProps {
  mode: 'create' | 'edit'
  state: NamespaceFormState
  onChange: (next: NamespaceFormState) => void
  onSubmit: (values: NamespaceFormState) => void
  /** When provided, Cancel calls this instead of `navigate(-1)`. */
  onCancel?: () => void
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

export default function NamespaceForm({
  mode,
  state,
  onChange,
  onSubmit,
  onCancel,
  isPending,
  errorMessage,
}: NamespaceFormProps) {
  const navigate = useNavigate()
  const [errors, setErrors] = useState(() => validateNamespaceForm(state, mode))
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

  const updateAction = (
    i: number,
    field: keyof NamespaceFormState['action_weights'][number],
    value: string | number,
  ) => {
    const rows = [...state.action_weights]
    rows[i] = { ...rows[i], [field]: value }
    propagate({ ...state, action_weights: rows })
  }

  const addAction = () => {
    propagate({
      ...state,
      action_weights: [...state.action_weights, { action: '', weight: 0 }],
    })
  }

  const removeAction = (i: number) => {
    propagate({
      ...state,
      action_weights: state.action_weights.filter((_, idx) => idx !== i),
    })
  }

  const handleCancel = () => {
    if (onCancel) onCancel()
    else navigate(-1)
  }

  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (isPending) return
    setSubmitted(true)
    const errs = validateNamespaceForm(state, mode)
    setErrors(errs)
    if (hasErrors(errs)) return
    onSubmit(state)
  }

  return (
    <Form onSubmit={submit}>
      {errorMessage ? (
        <Notice tone="fail" title="Save failed">
          {errorMessage}
        </Notice>
      ) : null}

      {mode === 'create' ? (
        <Section title="Identity">
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
              placeholder="prod"
              invalid={Boolean(errors.namespace)}
              autoFocus
            />
          </Field>
        </Section>
      ) : null}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Section
          title="Action weights"
          actions={<Badge>{state.action_weights.length}</Badge>}
        >
          {errors.action_weights ? (
            <div className="mb-3">
              <Notice tone="fail">{errors.action_weights}</Notice>
            </div>
          ) : null}
          <div className="flex flex-col gap-2">
            {state.action_weights.map((row, i) => (
              <div key={i} className="flex items-center gap-2">
                <Input
                  inputSize="sm"
                  value={row.action}
                  onChange={(e) => updateAction(i, 'action', e.target.value)}
                  placeholder="action name"
                  className="flex-1"
                  aria-label={`action name row ${i + 1}`}
                />
                <NumberInput
                  width="w-24"
                  value={row.weight}
                  onChange={(e) =>
                    updateAction(i, 'weight', Number(e.target.value))
                  }
                  step={0.1}
                  aria-label={`weight row ${i + 1}`}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => removeAction(i)}
                >
                  remove
                </Button>
              </div>
            ))}
            <div>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={addAction}
              >
                add action
              </Button>
            </div>
          </div>
        </Section>

        <Section title="Decay + scoring">
          <FormGrid columns={2}>
            <Field
              label="lambda (event decay rate)"
              htmlFor="ns-lambda"
              hint="e^(-λ × days_since)"
            >
              <NumberInput
                id="ns-lambda"
                value={state.lambda}
                onChange={(e) => update('lambda', Number(e.target.value))}
                step={0.01}
                min={0}
              />
            </Field>
            <Field
              label="gamma (object freshness)"
              htmlFor="ns-gamma"
              hint="Applied at rerank time."
            >
              <NumberInput
                id="ns-gamma"
                value={state.gamma}
                onChange={(e) => update('gamma', Number(e.target.value))}
                step={0.01}
                min={0}
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
                value={state.alpha}
                onChange={(e) => update('alpha', Number(e.target.value))}
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
                value={state.max_results}
                onChange={(e) => update('max_results', Number(e.target.value))}
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
                value={state.seen_items_days}
                onChange={(e) =>
                  update('seen_items_days', Number(e.target.value))
                }
                step={1}
                min={0}
                invalid={Boolean(errors.seen_items_days)}
              />
            </Field>
          </FormGrid>
        </Section>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Section title="Dense strategy">
          <Field label="Strategy" htmlFor="ns-strategy">
            <RadioGroup<DenseStrategy>
              name="ns-strategy"
              value={state.dense_strategy}
              onChange={(v) => update('dense_strategy', v)}
              options={STRATEGY_OPTIONS}
            />
          </Field>
          <div className="mt-3">
            <FormGrid columns={2}>
              <Field
                label="embedding dim"
                htmlFor="ns-dim"
                error={errors.embedding_dim}
                hint="Must match the strategy output (or your BYOE vectors)."
              >
                <NumberInput
                  id="ns-dim"
                  value={state.embedding_dim}
                  onChange={(e) =>
                    update('embedding_dim', Number(e.target.value))
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
        </Section>

        <Section title="Trending">
          <FormGrid columns={2}>
            <Field
              label="window (hours)"
              htmlFor="ns-tr-win"
              hint="Events older than this don't contribute."
              error={errors.trending_window}
            >
              <NumberInput
                id="ns-tr-win"
                value={state.trending_window}
                onChange={(e) =>
                  update('trending_window', Number(e.target.value))
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
                value={state.trending_ttl}
                onChange={(e) => update('trending_ttl', Number(e.target.value))}
                step={1}
                min={1}
                invalid={Boolean(errors.trending_ttl)}
              />
            </Field>
            <Field
              label="lambda trending"
              htmlFor="ns-tr-lambda"
              hint="Time-decay rate for the trending score."
            >
              <NumberInput
                id="ns-tr-lambda"
                value={state.lambda_trending}
                onChange={(e) =>
                  update('lambda_trending', Number(e.target.value))
                }
                step={0.01}
                min={0}
              />
            </Field>
          </FormGrid>
        </Section>
      </div>

      <div className="sticky bottom-0 -mb-6 bg-base border-t border-default py-3 flex justify-end gap-2 z-10">
        <Button type="button" variant="ghost" onClick={handleCancel}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" loading={isPending}>
          {mode === 'create' ? 'Create namespace' : 'Save changes'}
        </Button>
      </div>
    </Form>
  )
}
