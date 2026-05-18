import type { ReactNode } from 'react'

interface ToolbarProps {
  /** Accessible name for the toolbar region. Defaults to "Filters". */
  ariaLabel?: string
  children: ReactNode
}

// Filter + action row. Compact controls (h-8) sit naturally inside this.
// Uses role="toolbar" so screen readers announce the region; a custom label
// can disambiguate when more than one toolbar appears on a page.
export default function Toolbar({ ariaLabel = 'Filters', children }: ToolbarProps) {
  return (
    <div
      role="toolbar"
      aria-label={ariaLabel}
      className="flex flex-wrap items-center gap-2 border border-default rounded-sm bg-surface px-3 py-2.5"
    >
      {children}
    </div>
  )
}
