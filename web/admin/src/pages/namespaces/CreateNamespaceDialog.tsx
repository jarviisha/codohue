import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Alert,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  FormField,
  Inline,
  Input,
  NumberInput,
  Select,
  Stack,
} from '@jarviisha/davinci-react-ui'
import { useUpsertNamespace } from '@/services/namespaces'

const DENSE_SOURCES = [
  { value: 'disabled', label: 'disabled — sparse only' },
  { value: 'byoe', label: 'byoe — bring your own embeddings' },
  { value: 'item2vec', label: 'item2vec — cron retrains from events' },
  { value: 'svd', label: 'svd — cron retrains via matrix factorisation' },
  { value: 'catalog', label: 'catalog — auto-embed from ingested content' },
]

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export default function CreateNamespaceDialog({ open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} >
      {/*
       * Mount the body only while open so every field, the surfaced API key,
       * and the mutation state start fresh on each open — no effect-based reset
       * (which triggers cascading renders) needed.
       */}
      {open && <CreateNamespaceBody onOpenChange={onOpenChange} />}
    </Dialog>
  )
}

function CreateNamespaceBody({ onOpenChange }: Pick<Props, 'onOpenChange'>) {
  const navigate = useNavigate()
  const upsert = useUpsertNamespace()

  const [namespace, setNamespace] = useState('')
  const [denseSource, setDenseSource] = useState('disabled')
  const [embeddingDim, setEmbeddingDim] = useState(64)
  const [apiKeyShown, setApiKeyShown] = useState<string | null>(null)

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    upsert.mutate(
      {
        namespace,
        body: {
          dense_source: denseSource,
          embedding_dim: embeddingDim,
        },
      },
      {
        onSuccess: (data) => {
          if (data.api_key) {
            // First-create: the API key is returned once. Surface it here so
            // the operator can copy it before leaving the dialog.
            setApiKeyShown(data.api_key)
          } else {
            onOpenChange(false)
            navigate(`/ns/${encodeURIComponent(namespace)}`)
          }
        },
      },
    )
  }

  return (
    <>
      {apiKeyShown ? (
        <>
          <DialogHeader>
            <DialogTitle>Namespace created</DialogTitle>
            <DialogDescription>
              Copy the API key below — this is the only time it will be shown.
            </DialogDescription>
          </DialogHeader>
          <DialogContent>
            <Stack>
              <Alert
                variant="success"
                title={`#${namespace}`}
                description="API key (per-namespace data plane)"
              />
              <code className="font-mono text-sm break-all block">{apiKeyShown}</code>
            </Stack>
          </DialogContent>
          <DialogFooter>
            <Inline justify="end">
              <Button variant="ghost" onClick={() => onOpenChange(false)}>
                Close
              </Button>
              <Button onClick={() => navigate(`/ns/${encodeURIComponent(namespace)}`)}>
                Open namespace
              </Button>
            </Inline>
          </DialogFooter>
        </>
      ) : (
        <form onSubmit={onSubmit} className="contents">
          <DialogHeader>
            <DialogTitle>New namespace</DialogTitle>
            <DialogDescription>
              A namespace isolates events, vectors, and trending data for one tenant.
            </DialogDescription>
          </DialogHeader>
          <DialogContent>
            <Stack>
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

              <FormField label="Dense source" required>
                <Select value={denseSource} onChange={(e) => setDenseSource(e.target.value)}>
                  {DENSE_SOURCES.map((s) => (
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
                <NumberInput
                  value={embeddingDim}
                  onChange={(e) => setEmbeddingDim(Number(e.target.value))}
                  min={8}
                  max={2048}
                />
              </FormField>
            </Stack>
          </DialogContent>
          <DialogFooter>
            <Inline justify="end">
              <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={upsert.isPending || namespace.length === 0}>
                {upsert.isPending ? 'Creating…' : 'Create'}
              </Button>
            </Inline>
          </DialogFooter>
        </form>
      )}
    </>
  )
}
