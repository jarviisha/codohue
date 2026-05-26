import { useCallback, useEffect } from 'react'
import { useBlocker, type BlockerFunction } from 'react-router-dom'
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

type Props = {
  /** When true, intra-app navigation prompts before leaving. */
  dirty: boolean
  /** Optional override for the dialog title. */
  title?: string
  /** Optional override for the dialog description. */
  description?: string
}

/**
 * DirtyFormGuard blocks intra-SPA navigation and full-page unload while the
 * host form has unsaved changes.
 *
 *   - React Router v7 navigation → intercepted by useBlocker; we render a
 *     confirm dialog with Stay / Discard buttons that call proceed/reset.
 *   - Full-page reload / tab close / back-button → handled by a
 *     beforeunload listener that triggers the browser's native prompt.
 *
 * Place this once inside a form-owning page; pass `dirty` whenever the host
 * tracks a not-yet-submitted change.
 */
export default function DirtyFormGuard({
  dirty,
  title = 'Discard unsaved changes?',
  description = 'You have edits that have not been saved. Leaving this page will lose them.',
}: Props) {
  const shouldBlock = useCallback<BlockerFunction>(
    ({ currentLocation, nextLocation }) =>
      dirty && currentLocation.pathname !== nextLocation.pathname,
    [dirty],
  )

  const blocker = useBlocker(shouldBlock)

  // beforeunload covers the cases the router doesn't see — tab close, hard
  // reload, typing a new URL into the address bar. The browser shows its own
  // generic dialog; we just need to opt in by setting returnValue.
  useEffect(() => {
    if (!dirty) return
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [dirty])

  const isBlocked = blocker.state === 'blocked'

  return (
    <Dialog
      open={isBlocked}
      onOpenChange={(open) => {
        if (!open && blocker.state === 'blocked') blocker.reset()
      }}
      size="sm"
    >
      {isBlocked && (
        <>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          <DialogContent>
            <Stack gap="100">
              <Alert
                variant="warning"
                title="Heads up"
                description="Save first if you want to keep the current draft."
              />
            </Stack>
          </DialogContent>
          <DialogFooter>
            <Inline gap="100" justify="end">
              <Button variant="ghost" onClick={() => blocker.reset?.()}>
                Stay on page
              </Button>
              <Button tone="danger" onClick={() => blocker.proceed?.()}>
                Discard changes
              </Button>
            </Inline>
          </DialogFooter>
        </>
      )}
    </Dialog>
  )
}
