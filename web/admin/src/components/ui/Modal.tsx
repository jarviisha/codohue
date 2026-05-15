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

// 80ms opacity snap (no translate) per DESIGN.md §11. Esc closes; backdrop
// click closes; focus is NOT yet trapped — Phase 3 polishes a11y.
export default function Modal({ open, onClose, title, size = 'md', children, footer }: ModalProps) {
  const panelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose()
        return
      }

      if (e.key !== 'Tab') return
      const panel = panelRef.current
      if (!panel) return
      const focusable = Array.from(
        panel.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !el.hasAttribute('disabled') && !el.getAttribute('aria-hidden'))
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
    window.setTimeout(() => panelRef.current?.focus(), 0)

    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = previousOverflow
    }
  }, [open, onClose])

  if (!open) return null

  return createPortal(
    <div
      className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/60 p-4 transition-opacity duration-[80ms]"
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
