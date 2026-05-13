import { useEffect, useRef, useState, type ReactNode } from 'react'

interface DropdownProps {
  trigger: ReactNode
  align?: 'left' | 'right'
  children: ReactNode | ((close: () => void) => ReactNode)
}

// Small popover menu, opens on click. Closes on outside click, Escape, or
// when the children render-prop calls the supplied close().
export default function Dropdown({ trigger, align = 'left', children }: DropdownProps) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDocClick = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDocClick)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDocClick)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const close = () => setOpen(false)

  return (
    <div ref={rootRef} className="relative inline-block">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="inline-flex"
        aria-haspopup="menu"
        aria-expanded={open}
      >
        {trigger}
      </button>
      {open ? (
        <div
          role="menu"
          className={[
            'absolute top-full mt-1 min-w-40 rounded border border-default bg-surface shadow-overlay py-1 z-50',
            align === 'right' ? 'right-0' : 'left-0',
          ].join(' ')}
        >
          {typeof children === 'function' ? children(close) : children}
        </div>
      ) : null}
    </div>
  )
}

// Helper component for menu items inside a Dropdown.
export function DropdownItem({
  onSelect,
  children,
  destructive = false,
}: {
  onSelect: () => void
  children: ReactNode
  destructive?: boolean
}) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onSelect}
      className={[
        'w-full text-left px-3 py-1.5 text-sm hover:bg-surface-raised',
        destructive ? 'text-danger' : 'text-primary',
      ].join(' ')}
    >
      {children}
    </button>
  )
}
