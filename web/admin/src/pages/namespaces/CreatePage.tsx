import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Button,
  CodeBlock,
  Notice,
  PageHeader,
  PageShell,
  useRegisterCommand,
} from '../../components/ui'
import { ApiError } from '../../services/http'
import { useUpsertNamespace } from '../../services/namespaces'
import {
  defaultFormState,
  toUpsertPayload,
  type NamespaceFormState,
} from '../ns/configForm'
import NamespaceForm from '../ns/NamespaceForm'
import { paths } from '../../routes/path'

export default function NamespaceCreatePage() {
  const navigate = useNavigate()
  const upsert = useUpsertNamespace()

  const [state, setState] = useState<NamespaceFormState>(defaultFormState)

  // Holds the one-shot API key returned by the backend on first create.
  // When set, the page switches to a "key issued" view that must be
  // acknowledged before continuing to the namespace.
  const [createdKey, setCreatedKey] = useState<{
    namespace: string
    apiKey: string
  } | null>(null)

  useRegisterCommand(
    'namespaces.list',
    'Back to namespaces list',
    () => navigate(paths.namespaces),
    'global',
  )

  const handleSubmit = (values: NamespaceFormState) => {
    const payload = toUpsertPayload(values)
    upsert.mutate(
      { name: values.namespace, payload },
      {
        onSuccess: (res) => {
          if (res.api_key) {
            setCreatedKey({ namespace: res.namespace, apiKey: res.api_key })
          } else {
            // Defensive: backend should always return api_key on create,
            // but if it doesn't, just route to the namespace.
            navigate(paths.ns(res.namespace))
          }
        },
      },
    )
  }

  const errorMessage =
    upsert.error instanceof ApiError
      ? upsert.error.message
      : upsert.error instanceof Error
        ? upsert.error.message
        : undefined

  if (createdKey) {
    return (
      <PageShell>
        <PageHeader
          title={`Created "${createdKey.namespace}"`}
          meta="copy the API key before continuing — this is the only time it is shown"
          actions={
            <Button
              variant="ghost"
              size="sm"
              onClick={() => navigate(paths.namespaces)}
            >
              Namespaces
            </Button>
          }
        />
        <Notice tone="ok" title="API key issued">
          <p className="mb-3">
            Store this key somewhere safe. The admin server will never display
            it again; rotation requires a separate update.
          </p>
          <CodeBlock copyable language="api-key">
            {createdKey.apiKey}
          </CodeBlock>
        </Notice>
        <div className="flex justify-end">
          <Button
            variant="primary"
            onClick={() => navigate(paths.ns(createdKey.namespace))}
          >
            Continue to namespace
          </Button>
        </div>
      </PageShell>
    )
  }

  return (
    <PageShell>
      <PageHeader
        title="Create namespace"
        meta="An API key will be issued once on success."
        actions={
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate(paths.namespaces)}
          >
            Back
          </Button>
        }
      />
      <NamespaceForm
        mode="create"
        state={state}
        onChange={setState}
        onSubmit={handleSubmit}
        isPending={upsert.isPending}
        errorMessage={errorMessage}
      />
    </PageShell>
  )
}
