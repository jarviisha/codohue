import type { ReactNode } from 'react'

interface SidebarNavGroupProps {
  label: string
  children: ReactNode
}

// Section heading + items. Label is mono uppercase with terminal-tracking (§3.1).
export default function SidebarNavGroup({ label, children }: SidebarNavGroupProps) {
  return (
    <div className="px-2 py-3">
      <div className="px-3 py-1 font-mono text-[11px] uppercase tracking-[0.12em] text-muted">
        {label}
      </div>
      <nav className="flex flex-col gap-0.5 mt-1">{children}</nav>
    </div>
  )
}
