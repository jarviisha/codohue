import { useTriggerBatch } from '../../hooks/useTriggerBatch'
import { Button } from '../../components/ui'

export default function RunNowButton({ ns }: { ns: string }) {
  const trigger = useTriggerBatch(ns)

  async function handleClick() {
    try {
      await trigger.mutateAsync()
    } catch {
      // error shown below
    }
  }

  return (
    <Button
      onClick={handleClick}
      disabled={trigger.isPending}
      size="sm"
    >
      {trigger.isPending ? 'Running...' : 'Run now'}
    </Button>
  )
}
