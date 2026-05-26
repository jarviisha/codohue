import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Alert,
  Button,
  Card,
  CardContent,
  Container,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  FormField,
  Inline,
  Input,
  Stack,
} from '@jarviisha/davinci-react-ui'
import {
  useClearDemoData,
  useResetApp,
  useSeedDemoData,
  type DemoDatasetResponse,
  type ResetAppResponse,
} from '@/services/dangerZone'
import PageHeader from '@/components/shell/PageHeader'

/**
 * DangerZonePage gathers global destructive admin actions in one place so an
 * operator doesn't accidentally trigger them while drilling into a specific
 * namespace.
 *
 *   - Demo data seed / clear: idempotent helpers for the bundled demo.
 *   - App-wide reset: wipes every namespace. Guarded by a type-RESET dialog
 *     because the backend rejects any other body.
 */
export default function DangerZonePage() {
  const navigate = useNavigate()
  const seed = useSeedDemoData()
  const clear = useClearDemoData()
  const [resetOpen, setResetOpen] = useState(false)
  const [resetResult, setResetResult] = useState<ResetAppResponse | null>(null)

  return (
    <Container size="md" className="py-6">
      <PageHeader>
        <Stack gap="025">
          <h1 className="text-foreground text-xl font-semibold">Danger zone</h1>
          <p className="text-foreground-subtle text-sm">
            Destructive actions. Each runs against production data with no undo.
          </p>
        </Stack>
      </PageHeader>

      <Stack gap="300">
        {resetResult && (
          <Alert
            variant="success"
            title="App-wide reset complete"
            description={`Removed ${resetResult.namespaces_deleted} namespace(s) and ${resetResult.events_deleted.toLocaleString()} events.`}
            actions={
              <Button size="sm" variant="ghost" onClick={() => setResetResult(null)}>
                Dismiss
              </Button>
            }
          />
        )}

        <DemoCard
          title="Seed demo dataset"
          description="Creates the bundled demo namespace plus sample events and catalog items. Idempotent — re-runs reset the dataset back to its baseline state."
          action="Seed"
          tone="primary"
          loading={seed.isPending}
          error={seed.error}
          result={seed.data}
          onRun={() => seed.mutate()}
          onOpen={(ns) => navigate(`/ns/${encodeURIComponent(ns)}`)}
        />

        <DemoCard
          title="Clear demo dataset"
          description="Wipes the bundled demo namespace if present. Safe to run when the namespace doesn't exist."
          action="Clear"
          tone="danger"
          loading={clear.isPending}
          error={clear.error}
          result={clear.data}
          onRun={() => clear.mutate()}
        />

        <Card>
          <CardContent>
            <Stack gap="100">
              <span className="text-foreground-subtle text-xs uppercase tracking-wide">
                App-wide reset
              </span>
              <p className="text-foreground-subtle text-sm">
                Drops every namespace plus all data across Postgres, Redis, and Qdrant. Requires
                typing <code>RESET</code> to confirm.
              </p>
              <Inline gap="100" justify="end">
                <Button tone="danger" onClick={() => setResetOpen(true)}>
                  Reset everything…
                </Button>
              </Inline>
            </Stack>
          </CardContent>
        </Card>
      </Stack>

      <ResetAppDialog
        open={resetOpen}
        onOpenChange={setResetOpen}
        onSuccess={(result) => {
          setResetResult(result)
          setResetOpen(false)
        }}
      />
    </Container>
  )
}

function DemoCard({
  title,
  description,
  action,
  tone,
  loading,
  error,
  result,
  onRun,
  onOpen,
}: {
  title: string
  description: string
  action: string
  tone: 'primary' | 'danger'
  loading: boolean
  error: Error | null
  result: DemoDatasetResponse | undefined
  onRun: () => void
  onOpen?: (ns: string) => void
}) {
  return (
    <Card>
      <CardContent>
        <Stack gap="100">
          <span className="text-foreground-subtle text-xs uppercase tracking-wide">{title}</span>
          <p className="text-foreground-subtle text-sm">{description}</p>
          {error && (
            <Alert variant="danger" title={`${title} failed`} description={error.message} />
          )}
          {result && (
            <Alert
              variant="success"
              title={`${title} complete`}
              description={describeDemoResult(result)}
              actions={
                onOpen && (
                  <Button size="sm" variant="ghost" onClick={() => onOpen(result.namespace)}>
                    Open {result.namespace}
                  </Button>
                )
              }
            />
          )}
          <Inline gap="100" justify="end">
            <Button
              tone={tone === 'danger' ? 'danger' : undefined}
              onClick={onRun}
              disabled={loading}
            >
              {loading ? `${action.replace(/e$/, '')}ing…` : action}
            </Button>
          </Inline>
        </Stack>
      </CardContent>
    </Card>
  )
}

function describeDemoResult(r: DemoDatasetResponse): string {
  const bits: string[] = [`namespace ${r.namespace}`]
  if (r.events_created) bits.push(`${r.events_created.toLocaleString()} events created`)
  if (r.events_deleted) bits.push(`${r.events_deleted.toLocaleString()} events deleted`)
  if (r.catalog_items_created)
    bits.push(`${r.catalog_items_created.toLocaleString()} catalog items created`)
  if (r.api_key) bits.push('new api_key issued — copy from the response if you need it')
  return bits.join(' · ')
}

function ResetAppDialog({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: (result: ResetAppResponse) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} size="md">
      {open && <ResetAppForm onClose={() => onOpenChange(false)} onSuccess={onSuccess} />}
    </Dialog>
  )
}

function ResetAppForm({
  onClose,
  onSuccess,
}: {
  onClose: () => void
  onSuccess: (result: ResetAppResponse) => void
}) {
  const reset = useResetApp()
  const [confirm, setConfirm] = useState('')

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    reset.mutate(undefined, {
      onSuccess: (result) => onSuccess(result),
    })
  }

  return (
    <form onSubmit={onSubmit}>
      <DialogHeader>
        <DialogTitle>Reset everything</DialogTitle>
        <DialogDescription>
          Every namespace, every event, every vector — wiped across Postgres, Redis, and Qdrant.
          This cannot be undone.
        </DialogDescription>
      </DialogHeader>
      <DialogContent>
        <Stack gap="200">
          {reset.error && (
            <Alert variant="danger" title="Reset failed" description={reset.error.message} />
          )}
          <FormField
            label="Type RESET to confirm"
            required
            helpText="The backend rejects any other body. Case-sensitive."
          >
            <Input
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              placeholder="RESET"
              autoFocus
            />
          </FormField>
        </Stack>
      </DialogContent>
      <DialogFooter>
        <Inline gap="100" justify="end">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" tone="danger" disabled={confirm !== 'RESET' || reset.isPending}>
            {reset.isPending ? 'Resetting…' : 'Reset everything'}
          </Button>
        </Inline>
      </DialogFooter>
    </form>
  )
}
