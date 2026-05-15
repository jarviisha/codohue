import { Field, FormGrid, NumberInput, Panel } from '@/components/ui'
import type { TabProps } from './types'

export default function TrendingTab({ state, errors, updateNumber }: TabProps) {
  return (
    <Panel title="trending">
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
            onChange={(e) => updateNumber('trending_window', e.target.value)}
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
            onChange={(e) => updateNumber('trending_ttl', e.target.value)}
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
            onChange={(e) => updateNumber('lambda_trending', e.target.value)}
            step={0.01}
            min={0}
            invalid={Boolean(errors.lambda_trending)}
          />
        </Field>
      </FormGrid>
    </Panel>
  )
}
