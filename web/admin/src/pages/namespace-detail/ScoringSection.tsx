import { FormControl, NumberInput, Panel } from '../../components/ui'
import type { NamespaceFormState } from '../namespaceForm'
import type { UpdateNamespaceNumber } from './types'

export default function ScoringSection({
  form,
  onNumberChange,
}: {
  form: NamespaceFormState
  onNumberChange: UpdateNamespaceNumber
}) {
  return (
    <Panel title="Scoring Parameters" bodyClassName="flex flex-col gap-3">
      <FormControl label="Lambda (time decay)" htmlFor="scoring-lambda" inline>
        <NumberInput id="scoring-lambda" step="0.001" value={form.lambda} onChange={e => onNumberChange('lambda', e.target.value)} />
      </FormControl>
      <FormControl label="Gamma (object freshness)" htmlFor="scoring-gamma" inline>
        <NumberInput id="scoring-gamma" step="0.001" value={form.gamma} onChange={e => onNumberChange('gamma', e.target.value)} />
      </FormControl>
      <FormControl label="Alpha (CF blend)" htmlFor="scoring-alpha" inline>
        <NumberInput id="scoring-alpha" step="0.01" min={0} max={1} value={form.alpha} onChange={e => onNumberChange('alpha', e.target.value)} />
      </FormControl>
      <FormControl label="Max results" htmlFor="max-results" inline>
        <NumberInput id="max-results" min={1} value={form.max_results} onChange={e => onNumberChange('max_results', e.target.value)} />
      </FormControl>
      <FormControl label="Seen items days" htmlFor="seen-items-days" inline>
        <NumberInput id="seen-items-days" min={1} value={form.seen_items_days} onChange={e => onNumberChange('seen_items_days', e.target.value)} />
      </FormControl>
    </Panel>
  )
}
