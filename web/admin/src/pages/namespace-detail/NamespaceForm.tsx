import { useState } from 'react'
import { Button, FormControl, Panel, TextInput } from '../../components/ui'
import type { NamespaceFormState } from '../namespaceForm'
import ActionWeightsSection from './ActionWeightsSection'
import DenseHybridSection from './DenseHybridSection'
import ScoringSection from './ScoringSection'
import TrendingSection from './TrendingSection'

interface NamespaceFormProps {
  initialForm: NamespaceFormState
  isNew: boolean
  isPending: boolean
  onCancel: () => void
  onSubmit: (form: NamespaceFormState) => Promise<void>
}

export default function NamespaceForm({
  initialForm,
  isNew,
  isPending,
  onCancel,
  onSubmit,
}: NamespaceFormProps) {
  const [form, setForm] = useState(initialForm)

  function update<K extends keyof NamespaceFormState>(field: K, value: NamespaceFormState[K]) {
    setForm(current => ({ ...current, [field]: value }))
  }

  function updateNumber(field: keyof NamespaceFormState, value: string) {
    setForm(current => ({ ...current, [field]: Number(value) }))
  }

  function updateWeight(action: string, value: string) {
    setForm(current => ({
      ...current,
      action_weights: {
        ...current.action_weights,
        [action]: parseFloat(value),
      },
    }))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    await onSubmit(form)
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-6">
      {isNew && (
        <Panel title="Namespace Identity" bodyClassName="max-w-140">
          <FormControl label="Namespace name" htmlFor="namespace-name">
            <TextInput
              id="namespace-name"
              required
              value={form.name}
              onChange={e => update('name', e.target.value)}
              placeholder="e.g. my_feed"
              className="w-full"
            />
          </FormControl>
        </Panel>
      )}

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <ActionWeightsSection weights={form.action_weights} onChange={updateWeight} />
        <ScoringSection form={form} onNumberChange={updateNumber} />
        <DenseHybridSection form={form} onFieldChange={update} onNumberChange={updateNumber} />
        <TrendingSection form={form} onNumberChange={updateNumber} />
      </div>

      <div className="flex items-center gap-3 border-t border-default pt-4">
        <Button type="submit" variant="primary" disabled={isPending}>
          {isPending ? 'Saving...' : 'Save'}
        </Button>
        <Button type="button" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}
