import { FormControl, NumberInput, Panel } from '../../components/ui'
import type { NamespaceFormState } from '../namespaceForm'
import type { UpdateNamespaceNumber } from './types'

export default function TrendingSection({
  form,
  onNumberChange,
}: {
  form: NamespaceFormState
  onNumberChange: UpdateNamespaceNumber
}) {
  return (
    <Panel title="Trending" bodyClassName="flex flex-col gap-3">
      <FormControl label="Window (hours)" htmlFor="trending-window" inline>
        <NumberInput id="trending-window" min={1} value={form.trending_window} onChange={e => onNumberChange('trending_window', e.target.value)} />
      </FormControl>
      <FormControl label="TTL (seconds)" htmlFor="trending-ttl" inline>
        <NumberInput id="trending-ttl" min={0} value={form.trending_ttl} onChange={e => onNumberChange('trending_ttl', e.target.value)} />
      </FormControl>
      <FormControl label="Lambda trending" htmlFor="lambda-trending" inline>
        <NumberInput id="lambda-trending" step="0.01" value={form.lambda_trending} onChange={e => onNumberChange('lambda_trending', e.target.value)} />
      </FormControl>
    </Panel>
  )
}
