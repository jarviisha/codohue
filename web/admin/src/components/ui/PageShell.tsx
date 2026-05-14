import type { ReactNode } from 'react'

// Standard page wrapper. The AppShell already applies px-6 py-6 to its
// content slot, so PageShell mostly adds vertical rhythm between sections.
//
// The `gap-4` here is the single source of truth for spacing between the
// PageHeader and the first content section (and between subsequent siblings).
// Pages must not add their own margin-top / space-y to compensate — let the
// shell own that rhythm so every page looks the same.
export default function PageShell({ children }: { children: ReactNode }) {
  return <div className="flex flex-col gap-4">{children}</div>
}
