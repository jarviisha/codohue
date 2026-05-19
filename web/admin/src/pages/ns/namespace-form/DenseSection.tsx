import {
  Field,
  FormGrid,
  Notice,
  NumberInput,
  Panel,
  RadioGroup,
  Select,
} from '@/components/ui'
import type { RadioOption } from '@/components/ui'
import type { DenseStrategy } from '../configForm'
import type { SectionProps } from './types'

const STRATEGY_HINTS: Record<DenseStrategy, string> = {
  item2vec: 'Skip-gram trained item embeddings on co-occurrence.',
  svd: 'Truncated SVD over the interaction matrix.',
  byoe: 'Bring-your-own-embeddings via PUT /objects/:id/embedding.',
  disabled: 'Skip the dense phase entirely (sparse-only recommendations).',
}

// dense_strategy values that cron Phase 2 actually trains. These are the
// options the backend rejects (CatalogStrategyConflict, 400) when catalog
// auto-embedding is enabled, because both pipelines would race to write to
// {ns}_objects_dense.
const CRON_TRAINED_STRATEGIES: DenseStrategy[] = ['item2vec', 'svd']

const STRATEGY_ORDER: DenseStrategy[] = ['item2vec', 'svd', 'byoe', 'disabled']

export default function DenseSection({ state, errors, context, update, updateNumber }: SectionProps) {
  const catalogLock = context.catalogEnabled
  const strategyLabel = context.catalogStrategyId
    ? `${context.catalogStrategyId}${context.catalogStrategyVersion ? `@${context.catalogStrategyVersion}` : ''}`
    : null

  const strategyOptions: RadioOption<DenseStrategy>[] = STRATEGY_ORDER.map((value) => {
    const disabled = catalogLock && CRON_TRAINED_STRATEGIES.includes(value)
    return {
      value,
      label: value,
      hint: disabled
        ? `${STRATEGY_HINTS[value]} Disabled while catalog auto-embedding owns the dense vectors.`
        : STRATEGY_HINTS[value],
      disabled,
    }
  })

  return (
    <Panel title="dense strategy">
      {catalogLock ? (
        <div className="mb-4">
          <Notice tone="info" title="Locked by catalog auto-embedding">
            Catalog auto-embedding is enabled
            {strategyLabel ? (
              <>
                {' '}with <span className="font-mono text-primary">{strategyLabel}</span>
              </>
            ) : null}
            . The embedder pipeline owns dense vectors for this namespace, so{' '}
            <span className="font-mono">item2vec</span> and{' '}
            <span className="font-mono">svd</span> are unavailable and{' '}
            <span className="font-mono">embedding_dim</span> is managed by the catalog
            strategy.
          </Notice>
        </div>
      ) : null}

      <Field label="Strategy" htmlFor="ns-strategy">
        <RadioGroup<DenseStrategy>
          name="ns-strategy"
          value={state.dense_strategy}
          onChange={(v) => update('dense_strategy', v)}
          options={strategyOptions}
        />
      </Field>
      <div className="mt-4">
        <FormGrid columns={2}>
          <Field
            label="embedding dim"
            htmlFor="ns-dim"
            error={errors.embedding_dim}
            hint={
              catalogLock
                ? 'Managed by the catalog strategy. Change it from the Catalog config tab.'
                : 'Must match the strategy output (or your BYOE vectors).'
            }
          >
            <NumberInput
              id="ns-dim"
              width="w-36"
              value={state.embedding_dim}
              onChange={(e) => updateNumber('embedding_dim', e.target.value)}
              step={1}
              min={1}
              invalid={Boolean(errors.embedding_dim)}
              readOnly={catalogLock}
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
    </Panel>
  )
}
