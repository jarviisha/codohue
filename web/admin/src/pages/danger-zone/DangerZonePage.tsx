import { useState, type FormEvent } from 'react'
import {
  Banner,
  Button,
  Card,
  Dialog,
  DialogHeader,
  Layout,
  LayoutContent,
  LayoutFooter,
  Stack,
  TextInput,
} from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
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
    <PageContainer size="md">
      <PageHeader>
        <Stack gap={1}>
          <h1 className="text-primary text-xl font-semibold">Danger zone</h1>
          <p className="text-secondary text-sm">
            Destructive actions. Each runs against production data with no undo.
          </p>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        {resetResult && (
          <Banner
            status="success"
            title="App-wide reset complete"
            description={`Removed ${resetResult.namespaces_deleted} namespace(s) and ${resetResult.events_deleted.toLocaleString()} events.`}
            endContent={
              <Button size="sm" variant="ghost" onClick={() => setResetResult(null)} label="Dismiss" />
            }
          />
        )}

        <Card>
            <Stack gap={6}>
              <span className="text-secondary text-xs uppercase tracking-wide">
                App-wide reset
              </span>
              <p className="text-secondary text-sm">
                Drops every namespace plus all data across Postgres, Redis, and Qdrant. Requires
                typing <code>RESET</code> to confirm.
              </p>
              <Stack align="center" gap={4} direction="horizontal" justify="end">
                <Button variant="destructive"  onClick={() => setResetOpen(true)} label="Reset everything…" />
              </Stack>
            </Stack>
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
    </PageContainer>
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
    <Dialog isOpen={open} onOpenChange={onOpenChange} width={560} purpose="required">
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
    <form onSubmit={onSubmit} className="contents">
      <Layout
        header={
          <DialogHeader
            title="Reset everything"
            subtitle="Every namespace, every event, every vector — wiped across Postgres, Redis, and Qdrant. This cannot be undone."
            onOpenChange={onClose}
          />
        }
        content={
          <LayoutContent>
            <Stack gap={6}>
              {reset.error && (
                <Banner status="error" title="Reset failed" description={reset.error.message} />
              )}
              <TextInput
                label="Type RESET to confirm"
                description="The backend rejects any other body. Case-sensitive."
                value={confirm}
                onChange={setConfirm}
                placeholder="RESET"
              />
            </Stack>
          </LayoutContent>
        }
        footer={
          <LayoutFooter>
            <Stack direction="horizontal" gap={2} align="center" hAlign="end">
              <Button type="button" variant="ghost" onClick={onClose} label="Cancel" />
              <Button
                variant="destructive"
                type="submit"
                isDisabled={confirm !== 'RESET' || reset.isPending}
                label={reset.isPending ? 'Resetting…' : 'Reset everything'}
              />
            </Stack>
          </LayoutFooter>
        }
      />
    </form>
  )
}
