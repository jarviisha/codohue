import { FormControl, NumberInput, Panel } from '../../components/ui'
import type { NamespaceFormState } from '../namespaceForm'

export default function ActionWeightsSection({
  weights,
  onChange,
}: {
  weights: NamespaceFormState['action_weights']
  onChange: (action: string, value: string) => void
}) {
  return (
    <Panel title="Action Weights" bodyClassName="flex flex-col gap-3">
      {Object.entries(weights).map(([action, weight]) => {
        const id = `action-weight-${action.toLowerCase()}`
        return (
          <FormControl key={action} label={action} htmlFor={id} inline>
            <NumberInput
              id={id}
              step="0.1"
              value={weight}
              onChange={e => onChange(action, e.target.value)}
              className="w-20"
            />
          </FormControl>
        )
      })}
    </Panel>
  )
}
