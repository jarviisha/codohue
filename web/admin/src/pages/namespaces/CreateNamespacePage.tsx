import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Alert,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Container,
  FormField,
  Inline,
  Input,
  Select,
  Stack,
} from '@jarviisha/davinci-react-ui'
import { useUpsertNamespace } from '@/services/namespaces'
import DirtyFormGuard from '@/components/shell/DirtyFormGuard'

const DENSE_STRATEGIES = [
  { value: 'disabled', label: 'disabled — sparse only' },
  { value: 'byoe', label: 'byoe — bring your own embeddings' },
  { value: 'item2vec', label: 'item2vec — cron retrains from events' },
  { value: 'svd', label: 'svd — cron retrains via matrix factorisation' },
]

export default function CreateNamespacePage() {
  const navigate = useNavigate()
  const upsert = useUpsertNamespace()

  const [namespace, setNamespace] = useState('')
  const [denseStrategy, setDenseStrategy] = useState('disabled')
  const [embeddingDim, setEmbeddingDim] = useState(64)
  const [apiKeyShown, setApiKeyShown] = useState<string | null>(null)

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    upsert.mutate(
      {
        namespace,
        body: {
          dense_strategy: denseStrategy,
          embedding_dim: embeddingDim,
        },
      },
      {
        onSuccess: (data) => {
          if (data.api_key) {
            // First-create: API key is returned once. Surface it here so the
            // operator can copy it before navigating away.
            setApiKeyShown(data.api_key)
          } else {
            navigate(`/ns/${encodeURIComponent(namespace)}`)
          }
        },
      },
    )
  }

  // The form is "dirty" while the user has typed/tweaked anything but the
  // upsert hasn't either landed or been cancelled. Once apiKeyShown is set
  // we're past the form and the guard must be off so the success-screen
  // navigation buttons don't trip it.
  const isDirty =
    !apiKeyShown &&
    !upsert.isPending &&
    (namespace !== '' || denseStrategy !== 'disabled' || embeddingDim !== 64)

  if (apiKeyShown) {
    return (
      <Container size="sm" className="py-8">
        <Stack gap="300">
          <Alert
            variant="success"
            title="Namespace created"
            description="Copy the API key below — this is the only time it will be shown."
          />
          <Card>
            <CardHeader>
              <CardTitle>{namespace}</CardTitle>
              <CardDescription>API key (per-namespace data plane)</CardDescription>
            </CardHeader>
            <CardContent>
              <code className="font-mono text-sm break-all block">{apiKeyShown}</code>
            </CardContent>
          </Card>
          <Inline gap="100" justify="end">
            <Button variant="ghost" onClick={() => navigate('/namespaces')}>
              Back to list
            </Button>
            <Button onClick={() => navigate(`/ns/${encodeURIComponent(namespace)}`)}>
              Open namespace
            </Button>
          </Inline>
        </Stack>
      </Container>
    )
  }

  return (
    <Container size="sm" className="py-8">
      <DirtyFormGuard dirty={isDirty} />
      <Card>
        <CardHeader>
          <CardTitle>New namespace</CardTitle>
          <CardDescription>
            A namespace isolates events, vectors, and trending data for one tenant.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit}>
            <Stack gap="200">
              {upsert.error && (
                <Alert
                  variant="danger"
                  title="Could not create namespace"
                  description={upsert.error.message}
                />
              )}

              <FormField label="Namespace name" required>
                <Input
                  value={namespace}
                  onChange={(e) => setNamespace(e.target.value)}
                  pattern="[a-z0-9_-]+"
                  required
                  autoFocus
                  placeholder="e.g. prod"
                />
              </FormField>

              <FormField label="Dense strategy" required>
                <Select value={denseStrategy} onChange={(e) => setDenseStrategy(e.target.value)}>
                  {DENSE_STRATEGIES.map((s) => (
                    <option key={s.value} value={s.value}>
                      {s.label}
                    </option>
                  ))}
                </Select>
              </FormField>

              <FormField
                label="Embedding dim"
                helpText="Vector width for dense collections. 64 is a sane default for item2vec; 768 / 1024 typical for BYOE."
              >
                <Input
                  type="number"
                  value={embeddingDim}
                  onChange={(e) => setEmbeddingDim(Number(e.target.value))}
                  min={8}
                  max={2048}
                />
              </FormField>

              <Inline gap="100" justify="end">
                <Button variant="ghost" type="button" onClick={() => navigate('/namespaces')}>
                  Cancel
                </Button>
                <Button type="submit" disabled={upsert.isPending || namespace.length === 0}>
                  {upsert.isPending ? 'Creating…' : 'Create'}
                </Button>
              </Inline>
            </Stack>
          </form>
        </CardContent>
      </Card>
    </Container>
  )
}
