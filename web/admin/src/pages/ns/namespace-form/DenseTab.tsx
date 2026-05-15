import {
  Field,
  FormGrid,
  NumberInput,
  Panel,
  RadioGroup,
  Select,
} from '@/components/ui'
import type { RadioOption } from '@/components/ui'
import type { DenseStrategy } from '../configForm'
import type { TabProps } from './types'

const STRATEGY_OPTIONS: RadioOption<DenseStrategy>[] = [
  { value: 'item2vec', label: 'item2vec', hint: 'Skip-gram trained item embeddings on co-occurrence.' },
  { value: 'svd', label: 'svd', hint: 'Truncated SVD over the interaction matrix.' },
  { value: 'byoe', label: 'byoe', hint: 'Bring-your-own-embeddings via PUT /objects/:id/embedding.' },
  { value: 'disabled', label: 'disabled', hint: 'Skip the dense phase entirely (sparse-only recommendations).' },
]

export default function DenseTab({ state, errors, update, updateNumber }: TabProps) {
  return (
    <Panel title="dense strategy">
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
              onChange={(e) => updateNumber('embedding_dim', e.target.value)}
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
    </Panel>
  )
}
