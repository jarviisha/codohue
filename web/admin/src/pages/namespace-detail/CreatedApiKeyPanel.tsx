import { Button, Notice, Panel } from '../../components/ui'

export default function CreatedApiKeyPanel({
  apiKey,
  onDone,
}: {
  apiKey: string
  onDone: () => void
}) {
  return (
    <Panel>
      <Notice tone="success" role="status" className="mb-4">
        Namespace created. API key (shown once only):
      </Notice>
      <pre className="m-0 mb-4 break-all rounded border border-accent/20 bg-accent-subtle p-3 text-sm font-medium text-accent">
        {apiKey}
      </pre>
      <Button variant="primary" onClick={onDone}>
        Done
      </Button>
    </Panel>
  )
}
