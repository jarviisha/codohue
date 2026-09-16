import {
  Banner,
  Button,
  Dialog,
  DialogHeader,
  HStack,
  Layout,
  LayoutContent,
  LayoutFooter,
  VStack,
} from '@astryxdesign/core'
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
 *
 * Astryx ships AlertDialog for exactly this shape, but its description is a
 * plain string and there is no slot for the failure Banner, so the dialog is
 * composed by hand.
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
    <Dialog isOpen={open} onOpenChange={onOpenChange} width={420} purpose="required">
      <Layout
        header={<DialogHeader title={title} onOpenChange={onOpenChange} />}
        content={
          <LayoutContent>
            <VStack gap={4}>
              {description}
              {error && <Banner status="error" title="Action failed" description={error} />}
            </VStack>
          </LayoutContent>
        }
        footer={
          <LayoutFooter>
            <HStack gap={2} hAlign="end">
              <Button variant="ghost" label="Cancel" onClick={() => onOpenChange(false)} />
              <Button
                variant="destructive"
                label={pending ? 'Working…' : confirmLabel}
                onClick={onConfirm}
                isDisabled={pending}
              />
            </HStack>
          </LayoutFooter>
        }
      />
    </Dialog>
  )
}
