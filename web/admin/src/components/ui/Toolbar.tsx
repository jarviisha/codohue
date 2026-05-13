import type { ReactNode } from 'react'

// Filter + action row. Compact controls (h-8) sit naturally inside this.
export default function Toolbar({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2 border border-default rounded-sm bg-surface px-3 py-2.5">
      {children}
    </div>
  )
}
