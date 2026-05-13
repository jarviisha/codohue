import type { ReactNode } from 'react'

// Terminal-style key cap for shortcut hints. Mono font, bordered.
export default function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="font-mono text-[11px] inline-flex items-center h-5 px-1.5 rounded-sm border border-default bg-surface text-secondary">
      {children}
    </kbd>
  )
}
