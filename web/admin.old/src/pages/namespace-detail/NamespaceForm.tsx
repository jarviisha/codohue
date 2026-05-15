import { useMemo, useState } from 'react'
import { Badge, Button, FormControl, Panel, TextInput } from '../../components/ui'
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
  const initialSignature = useMemo(() => formSignature(initialForm), [initialForm])
  const currentSignature = useMemo(() => formSignature(form), [form])
  const isDirty = currentSignature !== initialSignature

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
    if (!isDirty && !isNew) return
    await onSubmit(form)
  }

  function handleReset() {
    setForm(initialForm)
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

      <div className="sticky bottom-0 z-10 -mx-1 rounded border border-default bg-surface/95 p-3 shadow-floating backdrop-blur">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2">
            <Badge tone={isDirty || isNew ? 'warning' : 'success'} dot>
              {isDirty || isNew ? 'Unsaved changes' : 'Saved'}
            </Badge>
            <span className="text-xs text-muted">
              {isDirty || isNew
                ? 'Review and save changes before leaving settings.'
                : 'No pending changes.'}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="ghost"
              disabled={!isDirty || isPending}
              onClick={handleReset}
            >
              Reset
            </Button>
            <Button type="button" onClick={onCancel} disabled={isPending}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={isPending || (!isDirty && !isNew)}>
              {isPending ? 'Saving...' : 'Save'}
            </Button>
          </div>
        </div>
      </div>
    </form>
  )
}

function formSignature(form: NamespaceFormState) {
  return JSON.stringify({
    ...form,
    action_weights: Object.fromEntries(
      Object.entries(form.action_weights).sort(([a], [b]) => a.localeCompare(b)),
    ),
  })
}
