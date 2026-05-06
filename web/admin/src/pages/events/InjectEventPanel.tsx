import { useState } from 'react'
import ErrorBanner from '../../components/ErrorBanner'
import { Button, FormControl, Notice, Panel, Select, TextInput, Toolbar } from '../../components/ui'

export interface InjectEventPayload {
  subject_id: string
  object_id: string
  action: string
}

export default function InjectEventPanel({
  actions,
  errorMessage,
  isPending,
  onInject,
}: {
  actions: string[]
  errorMessage?: string
  isPending: boolean
  onInject: (payload: InjectEventPayload) => Promise<void>
}) {
  const [subject, setSubject] = useState('')
  const [object, setObject] = useState('')
  const [action, setAction] = useState('VIEW')
  const [success, setSuccess] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!subject.trim() || !object.trim()) return
    try {
      await onInject({
        subject_id: subject.trim(),
        object_id: object.trim(),
        action,
      })
      setSuccess(true)
      setSubject('')
      setObject('')
      setTimeout(() => setSuccess(false), 3000)
    } catch {
      // error shown via errorMessage
    }
  }

  return (
    <Panel title="Inject Test Event" className="mb-6">
      {errorMessage && <ErrorBanner message={errorMessage} />}
      {success && (
        <Notice tone="success" role="status" className="mb-4">
          Event injected successfully.
        </Notice>
      )}
      <form onSubmit={handleSubmit}>
        <Toolbar>
          <FormControl
            label="Subject ID"
            htmlFor="inject-subject-id"
            labelClassName="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted"
          >
            <TextInput
              id="inject-subject-id"
              value={subject}
              onChange={e => setSubject(e.target.value)}
              placeholder="user-1"
              className="w-48 py-2.5"
              required
            />
          </FormControl>
          <FormControl
            label="Object ID"
            htmlFor="inject-object-id"
            labelClassName="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted"
          >
            <TextInput
              id="inject-object-id"
              value={object}
              onChange={e => setObject(e.target.value)}
              placeholder="item-42"
              className="w-48 py-2.5"
              required
            />
          </FormControl>
          <FormControl
            label="Action"
            htmlFor="inject-action"
            labelClassName="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted"
          >
            <Select
              id="inject-action"
              value={action}
              onChange={e => setAction(e.target.value)}
              className="py-2.5"
            >
              {actions.map(a => <option key={a} value={a}>{a}</option>)}
            </Select>
          </FormControl>
          <Button
            type="submit"
            variant="primary"
            disabled={isPending || !subject.trim() || !object.trim()}
          >
            {isPending ? 'Injecting...' : 'Inject Event'}
          </Button>
        </Toolbar>
      </form>
    </Panel>
  )
}
