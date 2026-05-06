import { FormControl, FormSection, NumberInput, Select } from '../../components/ui'
import type { NamespaceFormState } from '../namespaceForm'
import type { UpdateNamespaceField, UpdateNamespaceNumber } from './types'

export default function DenseHybridSection({
  form,
  onFieldChange,
  onNumberChange,
}: {
  form: NamespaceFormState
  onFieldChange: UpdateNamespaceField
  onNumberChange: UpdateNamespaceNumber
}) {
  return (
    <FormSection title="Dense Hybrid">
      <FormControl label="Strategy" htmlFor="dense-strategy">
        <Select id="dense-strategy" value={form.dense_strategy} onChange={e => onFieldChange('dense_strategy', e.target.value)} className="w-full">
          <option value="item2vec">item2vec</option>
          <option value="svd">svd</option>
          <option value="byoe">byoe</option>
          <option value="disabled">disabled</option>
        </Select>
      </FormControl>
      <FormControl label="Embedding dim" htmlFor="embedding-dim" inline>
        <NumberInput id="embedding-dim" min={1} value={form.embedding_dim} onChange={e => onNumberChange('embedding_dim', e.target.value)} />
      </FormControl>
      <FormControl label="Distance" htmlFor="dense-distance">
        <Select id="dense-distance" value={form.dense_distance} onChange={e => onFieldChange('dense_distance', e.target.value)} className="w-full">
          <option value="cosine">cosine</option>
          <option value="dot">dot</option>
        </Select>
      </FormControl>
    </FormSection>
  )
}
