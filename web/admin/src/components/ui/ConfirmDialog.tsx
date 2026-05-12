import type { ReactNode } from 'react'
import Button from './Button'
import Modal from './Modal'

interface ConfirmDialogProps {
  open: boolean
  title: ReactNode
  children: ReactNode
  confirmLabel: string
  cancelLabel?: string
  tone?: 'primary' | 'danger'
  isPending?: boolean
  onCancel: () => void
  onConfirm: () => void
}

export default function ConfirmDialog({
  open,
  title,
  children,
  confirmLabel,
  cancelLabel = 'Cancel',
  tone = 'primary',
  isPending = false,
  onCancel,
  onConfirm,
}: ConfirmDialogProps) {
  return (
    <Modal open={open} onClose={isPending ? () => null : onCancel} title={title}>
      <div className="flex flex-col gap-4">
        <div className="text-sm leading-6 text-secondary">
          {children}
        </div>
        <div className="flex justify-end gap-2 border-t border-default pt-4">
          <Button
            type="button"
            variant="secondary"
            disabled={isPending}
            onClick={onCancel}
          >
            {cancelLabel}
          </Button>
          <Button
            type="button"
            variant={tone === 'danger' ? 'danger' : 'primary'}
            disabled={isPending}
            onClick={onConfirm}
          >
            {isPending ? 'Working...' : confirmLabel}
          </Button>
        </div>
      </div>
    </Modal>
  )
}
