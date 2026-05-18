import { useState, type FormEvent } from 'react'
import {
  Button,
  Field,
  Form,
  Input,
  Modal,
  Notice,
  Select,
} from '@/components/ui'
import {
  EVENT_ACTIONS,
  useInjectEvent,
  type EventAction,
  type InjectEventRequest,
} from '@/services/events'

interface InjectEventModalProps {
  namespace: string
  open: boolean
  onClose: () => void
}

interface FormState {
  subject_id: string
  object_id: string
  action: EventAction
  occurred_at: string
}

const EMPTY_FORM: FormState = {
  subject_id: '',
  object_id: '',
  action: 'VIEW',
  occurred_at: '',
}

const FORM_ID = 'inject-event-form'

// Modal for POST /api/admin/v1/namespaces/{ns}/events. The data plane fills
// in occurred_at with the current time when the field is omitted; the input
// is exposed so operators can backfill ad-hoc test events.
export default function InjectEventModal({ namespace, open, onClose }: InjectEventModalProps) {
  const inject = useInjectEvent()
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [error, setError] = useState<string | null>(null)

  const reset = () => {
    setForm(EMPTY_FORM)
    setError(null)
    inject.reset()
  }

  const handleClose = () => {
    reset()
    onClose()
  }

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!form.subject_id.trim()) {
      setError('subject_id is required')
      return
    }
    if (!form.object_id.trim()) {
      setError('object_id is required')
      return
    }
    setError(null)

    const payload: InjectEventRequest = {
      subject_id: form.subject_id.trim(),
      object_id: form.object_id.trim(),
      action: form.action,
    }
    if (form.occurred_at.trim()) {
      payload.occurred_at = form.occurred_at.trim()
    }

    inject.mutate(
      { namespace, payload },
      { onSuccess: handleClose },
    )
  }

  return (
    <Modal
      open={open}
      onClose={handleClose}
      size="md"
      title={`inject event · ${namespace}`}
      footer={
        <>
          <Button variant="ghost" onClick={handleClose}>Cancel</Button>
          <Button
            variant="primary"
            type="submit"
            form={FORM_ID}
            loading={inject.isPending}
          >
            Inject
          </Button>
        </>
      }
    >
      <Form id={FORM_ID} onSubmit={submit}>
        {error ? (
          <Notice tone="fail" title="Invalid payload">{error}</Notice>
        ) : null}
        {inject.isError ? (
          <Notice tone="fail" title="Inject failed">
            {(inject.error as Error)?.message ?? 'Unable to inject event.'}
          </Notice>
        ) : null}

        <Field label="subject_id" htmlFor="inject-event-subject-id" required>
          <Input
            id="inject-event-subject-id"
            value={form.subject_id}
            placeholder="user_19283"
            autoFocus
            onChange={(event) => setForm((f) => ({ ...f, subject_id: event.target.value }))}
          />
        </Field>
        <Field label="object_id" htmlFor="inject-event-object-id" required>
          <Input
            id="inject-event-object-id"
            value={form.object_id}
            placeholder="sku_42"
            onChange={(event) => setForm((f) => ({ ...f, object_id: event.target.value }))}
          />
        </Field>
        <Field label="action" htmlFor="inject-event-action" required>
          <Select
            id="inject-event-action"
            value={form.action}
            onChange={(event) =>
              setForm((f) => ({ ...f, action: event.target.value as EventAction }))
            }
          >
            {EVENT_ACTIONS.map((value) => (
              <option key={value} value={value}>{value}</option>
            ))}
          </Select>
        </Field>
        <Field
          label="occurred_at"
          htmlFor="inject-event-occurred-at"
          hint="Optional RFC3339 timestamp. Defaults to now when blank."
        >
          <Input
            id="inject-event-occurred-at"
            value={form.occurred_at}
            placeholder="2026-05-18T14:02:38Z"
            onChange={(event) => setForm((f) => ({ ...f, occurred_at: event.target.value }))}
          />
        </Field>
      </Form>
    </Modal>
  )
}
