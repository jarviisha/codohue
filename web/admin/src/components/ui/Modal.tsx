import { useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

interface ModalProps {
  open: boolean
  onClose: () => void
  title?: ReactNode
  size?: 'sm' | 'md' | 'lg'
  children: ReactNode
  footer?: ReactNode
}

const SIZE: Record<NonNullable<ModalProps['size']>, string> = {
  sm: 'max-w-sm',
  md: 'max-w-lg',
  lg: 'max-w-3xl',
}

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'

function getFocusable(panel: HTMLElement): HTMLElement[] {
  return Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (el) => !el.hasAttribute('disabled') && !el.getAttribute('aria-hidden'),
  )
}

// 80ms opacity snap (no translate) per DESIGN.md §11. Esc closes; backdrop
// click closes. Tab/Shift-Tab cycle within the panel; on open the first
// content-focusable receives focus (skipping the header close button); on
// close, focus is restored to the element that opened the modal.
export default function Modal({ open, onClose, title, size = 'md', children, footer }: ModalProps) {
  const panelRef = useRef<HTMLDivElement>(null)

  // Hold the latest onClose in a ref so the effect below only depends on
  // `open` — most callers pass inline arrows, which would otherwise re-fire
  // the effect on every parent render and thrash focus restoration.
  const onCloseRef = useRef(onClose)
  useEffect(() => {
    onCloseRef.current = onClose
  }, [onClose])

  useEffect(() => {
    if (!open) return

    // Capture the element that triggered the modal so we can restore focus
    // on close. activeElement is null at most ~never; the cast keeps the
    // restore branch terse.
    const trigger = document.activeElement as HTMLElement | null

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onCloseRef.current()
        return
      }

      if (e.key !== 'Tab') return
      const panel = panelRef.current
      if (!panel) return
      const focusable = getFocusable(panel)
      if (focusable.length === 0) {
        e.preventDefault()
        panel.focus()
        return
      }
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }

    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    document.addEventListener('keydown', onKey)

    // First-focus: defer to the next tick so React's autoFocus on body
    // inputs has already fired. If anything inside the panel already has
    // focus (e.g. an autoFocus input), leave it alone; otherwise pick the
    // first content focusable, skipping the header Close button so the
    // operator does not land on "dismiss" by default.
    window.setTimeout(() => {
      const panel = panelRef.current
      if (!panel) return
      if (panel.contains(document.activeElement)) return
      const focusable = getFocusable(panel)
      const target = focusable.find((el) => el.getAttribute('aria-label') !== 'Close')
      ;(target ?? panel).focus()
    }, 0)

    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = previousOverflow
      // Restore focus to the trigger if it is still mounted and focusable.
      // Skipping the restore when the trigger has been unmounted (e.g. a
      // row-level button that disappeared after the action) lets the browser
      // fall back to body, which is the standard no-op behaviour.
      if (trigger && document.body.contains(trigger) && typeof trigger.focus === 'function') {
        trigger.focus()
      }
    }
  }, [open])

  if (!open) return null

  return createPortal(
    <div
      className="fixed inset-0 z-9999 flex items-center justify-center bg-black/60 p-4 transition-opacity duration-80"
      onClick={onClose}
      role="presentation"
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
        className={`flex max-h-[calc(100dvh-2rem)] w-full flex-col overflow-hidden bg-surface border border-default rounded shadow-overlay focus:outline-none ${SIZE[size]}`}
        onClick={(e) => e.stopPropagation()}
      >
        {title ? (
          <header className="flex shrink-0 items-center justify-between gap-3 px-4 py-3 border-b border-default">
            <h2 className="min-w-0 truncate text-sm font-semibold text-primary">{title}</h2>
            <button
              type="button"
              onClick={onClose}
              className="h-6 px-2 flex items-center justify-center font-mono text-xs uppercase tracking-[0.06em] text-muted hover:text-primary rounded-sm"
              aria-label="Close"
            >
              close
            </button>
          </header>
        ) : null}
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4">{children}</div>
        {footer ? (
          <footer className="shrink-0 px-4 py-3 border-t border-default flex justify-end gap-2">
            {footer}
          </footer>
        ) : null}
      </div>
    </div>,
    document.body,
  )
}
