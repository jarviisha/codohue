import { Field, Input, Panel } from '@/components/ui'
import { normalizeNamespaceName } from '../configForm'
import type { TabProps } from './types'

export default function IdentityTab({ state, errors, update }: TabProps) {
  return (
    <Panel title="identity">
      <Field
        label="Namespace name"
        htmlFor="ns-name"
        required
        error={errors.namespace}
        hint="Lowercase letters, digits, underscore, dash. This is permanent."
      >
        <Input
          id="ns-name"
          value={state.namespace}
          onChange={(e) => update('namespace', e.target.value)}
          onBlur={(e) => update('namespace', normalizeNamespaceName(e.target.value))}
          placeholder="prod"
          invalid={Boolean(errors.namespace)}
          autoFocus
        />
      </Field>
    </Panel>
  )
}
