import {
  Alert,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Inline,
  Stack,
} from '@jarviisha/davinci-react-ui'
import type { ReactNode } from 'react'

/**
 * ConfirmDialog gates a one-click destructive action behind an explicit
 * confirmation.
 *
 * Deliberately lighter than the type-RESET dialog on the Danger zone page:
 * that guards an app-wide wipe, this guards a single-row delete where a typing
 * ritual would be friction without safety. Anything that removes data from
 * more than one store still belongs behind the typed variant.
 *
 * The action stays owned by the caller — this component only decides *whether*
 * it runs, so the mutation's pending / error state renders where it belongs.
 */
export default function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  onConfirm,
  pending = false,
  error,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: ReactNode
  confirmLabel: string
  onConfirm: () => void
  pending?: boolean
  error?: string
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} size="sm">
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      {error && (
        <DialogContent>
          <Stack>
            <Alert variant="danger" title="Action failed" description={error} />
          </Stack>
        </DialogContent>
      )}
      <DialogFooter>
        <Inline justify="end">
          <Button variant="ghost" tone="neutral" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button tone="danger" onClick={onConfirm} disabled={pending}>
            {pending ? 'Working…' : confirmLabel}
          </Button>
        </Inline>
      </DialogFooter>
    </Dialog>
  )
}
