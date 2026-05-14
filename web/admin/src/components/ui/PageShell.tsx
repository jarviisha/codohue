import type { ReactNode } from 'react'

// Standard page wrapper. The AppShell already applies px-6 py-6 to its
// content slot, so PageShell mostly adds vertical rhythm between sections.
export default function PageShell({ children }: { children: ReactNode }) {
  return <div className="flex flex-col">{children}</div>
}
