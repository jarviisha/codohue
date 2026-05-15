import type { ReactNode } from 'react'

// Inline mono badge for IDs, slugs, hashes. Subtle bg, small radius.
export default function CodeBadge({ children }: { children: ReactNode }) {
  return (
    <code className="font-mono text-xs px-1.5 py-0.5 rounded-sm bg-surface-raised text-primary">
      {children}
    </code>
  )
}
