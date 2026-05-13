import { useEffect, type ReactNode } from 'react'

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
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center px-4 bg-black/50 transition-opacity duration-[80ms]"
      onClick={onClose}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        className={`bg-surface border border-default rounded shadow-overlay w-full ${SIZE[size]}`}
        onClick={(e) => e.stopPropagation()}
      >
        {title ? (
          <header className="flex items-center justify-between px-4 py-3 border-b border-default">
            <h2 className="text-sm font-semibold text-primary">{title}</h2>
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
        <div className="p-4">{children}</div>
        {footer ? (
          <footer className="px-4 py-3 border-t border-default flex justify-end gap-2">
            {footer}
          </footer>
        ) : null}
      </div>
    </div>
  )
}
