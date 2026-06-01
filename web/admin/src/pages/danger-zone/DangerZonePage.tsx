import { useState, type FormEvent } from 'react'
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
import { useResetApp, type ResetAppResponse } from '@/services/dangerZone'
import PageHeader from '@/components/shell/PageHeader'

/**
 * DangerZonePage gathers global destructive admin actions in one place so an
 * operator doesn't accidentally trigger them while drilling into a specific
 * namespace. Currently the app-wide reset, guarded by a type-RESET dialog
 * because the backend rejects any other body. (Non-destructive demo seeding
 * lives on its own /demo-data page.)
 */
export default function DangerZonePage() {
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
