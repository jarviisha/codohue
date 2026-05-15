import { Badge, Button, Input, Notice, NumberInput, Panel } from '@/components/ui'
import {
  defaultActionWeights,
  type ActionWeightRow,
  type NamespaceFormState,
} from '../configForm'
import type { TabProps } from './types'

type Props = TabProps & {
  setActions: (rows: ActionWeightRow[]) => void
}

export default function ActionsTab({ state, errors, setActions }: Props) {
  const rows = state.action_weights

  const updateField = <K extends keyof ActionWeightRow>(
    i: number,
    field: K,
    value: ActionWeightRow[K],
  ) => {
    const next = [...rows]
    next[i] = { ...next[i], [field]: value }
    setActions(next)
  }

  const updateWeight = (i: number, raw: string) => {
    if (raw.trim() === '') return
    const next = Number(raw)
    if (!Number.isFinite(next)) return
    updateField(i, 'weight', next)
  }

  const add = () => setActions([...rows, { action: '', weight: 0 }])
  const remove = (i: number) => setActions(rows.filter((_, idx) => idx !== i))
  const resetDefault = () =>
    setActions(defaultActionWeights.map((row: NamespaceFormState['action_weights'][number]) => ({ ...row })))

  return (
    <Panel
      title="action weights"
      actions={
        <>
          <Button type="button" variant="ghost" size="xs" onClick={resetDefault}>
            reset default
          </Button>
          <Button type="button" variant="secondary" size="xs" onClick={add}>
            add action
          </Button>
        </>
      }
    >
      <div className="flex items-center gap-2 mb-4">
        <Badge>{rows.length}</Badge>
        <span className="text-sm text-secondary">configured action weights</span>
      </div>

      {errors.action_weights ? (
        <div className="mb-3">
          <Notice tone="fail">{errors.action_weights}</Notice>
        </div>
      ) : null}

      <div className="hidden sm:grid grid-cols-[minmax(0,1fr)_8rem_5.5rem] gap-2 px-1 pb-2 border-b border-default">
        <div className="font-mono text-xs uppercase tracking-[0.04em] text-secondary">action</div>
        <div className="font-mono text-xs uppercase tracking-[0.04em] text-secondary text-right">weight</div>
        <div className="font-mono text-xs uppercase tracking-[0.04em] text-secondary text-right">row</div>
      </div>

      <div className="flex flex-col gap-2 mt-2">
        {rows.map((row, i) => (
          <div
            key={i}
            className="grid grid-cols-1 sm:grid-cols-[minmax(0,1fr)_8rem_5.5rem] gap-2 items-center"
          >
            <Input
              inputSize="sm"
              value={row.action}
              onChange={(e) => updateField(i, 'action', e.target.value)}
              placeholder="action name"
              className="w-full"
              aria-label={`action name row ${i + 1}`}
            />
            <NumberInput
              width="w-full"
              value={row.weight}
              onChange={(e) => updateWeight(i, e.target.value)}
              step={0.1}
              aria-label={`weight row ${i + 1}`}
            />
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="sm:justify-self-end"
              onClick={() => remove(i)}
            >
              remove
            </Button>
          </div>
        ))}
      </div>
    </Panel>
  )
}
