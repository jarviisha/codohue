import { Field, FormGrid, NumberInput, Panel } from '@/components/ui'
import type { TabProps } from './types'

export default function ScoringTab({ state, errors, updateNumber }: TabProps) {
  return (
    <Panel title="decay + scoring">
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
        <Field label="max results" htmlFor="ns-maxr" error={errors.max_results}>
          <NumberInput
            id="ns-maxr"
            width="w-36"
            value={state.max_results}
            onChange={(e) => updateNumber('max_results', e.target.value)}
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
            onChange={(e) => updateNumber('seen_items_days', e.target.value)}
            step={1}
            min={0}
            invalid={Boolean(errors.seen_items_days)}
          />
        </Field>
      </FormGrid>
    </Panel>
  )
}
