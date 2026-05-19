import { useState, type ReactNode } from 'react'
import Modal from './Modal'
import Button from './Button'
import Input from './Input'

interface ConfirmDialogProps {
  open: boolean
  title: ReactNode
  description?: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
  loading?: boolean
  onConfirm: () => void
  onCancel: () => void
  // When set, the confirm button stays disabled until the operator types this
  // exact phrase. Used for destructive operations (delete namespace, reset
  // app) so a misclick can't fire them.
  requireTyped?: string
}

// Destructive actions (delete, drop, force-clear) must route through this
// dialog. The destructive prop swaps the confirm button to the danger
// variant. Pass requireTyped to force the operator to retype a phrase
// (typically the namespace name or a literal sentinel like "RESET") before
// the confirm button activates.
export default function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  destructive = false,
  loading = false,
  onConfirm,
  onCancel,
  requireTyped,
}: ConfirmDialogProps) {
  const [typed, setTyped] = useState('')

  // Reset the typed phrase every time the dialog re-opens (or the required
  // phrase changes while open) so a previous confirmation can't carry over.
  // Adjust state during render rather than in an effect — see the React docs
  // pattern for "resetting state when a prop changes".
  const [prevOpen, setPrevOpen] = useState(open)
  const [prevRequireTyped, setPrevRequireTyped] = useState(requireTyped)
  if (open !== prevOpen || requireTyped !== prevRequireTyped) {
    setPrevOpen(open)
    setPrevRequireTyped(requireTyped)
    if (open) setTyped('')
  }

  const typedMatches = requireTyped == null || typed === requireTyped
  const confirmDisabled = loading || !typedMatches

  return (
    <Modal
      open={open}
      onClose={onCancel}
      title={title}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onCancel} disabled={loading}>
            {cancelLabel}
          </Button>
          <Button
            variant={destructive ? 'danger' : 'primary'}
            onClick={onConfirm}
            loading={loading}
            disabled={confirmDisabled}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      {description ? <p className="text-sm text-secondary">{description}</p> : null}
      {requireTyped != null ? (
        <div className="mt-4 flex flex-col gap-2">
          <label className="text-xs uppercase tracking-[0.06em] text-muted" htmlFor="confirm-dialog-typed">
            Type <span className="font-mono text-primary">{requireTyped}</span> to confirm
          </label>
          <Input
            id="confirm-dialog-typed"
            autoFocus
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            placeholder={requireTyped}
            autoComplete="off"
            spellCheck={false}
          />
        </div>
      ) : null}
    </Modal>
  )
}
