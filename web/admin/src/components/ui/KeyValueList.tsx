import type { ReactNode } from 'react'

export interface KeyValueRow {
  label: ReactNode
  value: ReactNode
}

interface KeyValueListProps {
  rows: KeyValueRow[]
}

// Inline definition-list rows for dense settings panels. Labels left-aligned
// mono, values right-aligned mono tabular-nums.
export default function KeyValueList({ rows }: KeyValueListProps) {
  return (
    <dl className="flex flex-col gap-2">
      {rows.map((row, i) => (
        <div
          key={i}
          className="flex items-baseline justify-between gap-4 text-sm leading-5"
        >
          <dt className="font-mono text-secondary">{row.label}</dt>
          <dd className="font-mono tabular-nums text-primary text-right">{row.value}</dd>
        </div>
      ))}
    </dl>
  )
}
