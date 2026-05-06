export type StatusFilter = 'all' | 'running' | 'ok' | 'failed'

export default function FilterChips({
  value,
  onChange,
  counts,
}: {
  value: StatusFilter
  onChange: (v: StatusFilter) => void
  counts: Record<StatusFilter, number>
}) {
  const chips: { key: StatusFilter; label: string }[] = [
    { key: 'all', label: 'All' },
    { key: 'running', label: 'Running' },
    { key: 'ok', label: 'OK' },
    { key: 'failed', label: 'Failed' },
  ]
  return (
    <div className="flex flex-wrap gap-1.5 rounded-lg border border-default bg-surface p-1 w-fit">
      {chips.map(({ key, label }) => (
        <button
          key={key}
          type="button"
          aria-pressed={value === key}
          onClick={() => onChange(key)}
          className={[
            'cursor-pointer rounded border px-2.5 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:shadow-focus',
            value === key
              ? 'border-accent/20 bg-accent-subtle text-accent'
              : 'border-transparent bg-transparent text-secondary hover:bg-surface-raised hover:text-primary',
          ].join(' ')}
        >
          {label}
          {counts[key] > 0 && (
            <span className={['ml-1.5 tabular-nums', value === key ? 'opacity-80' : 'text-muted'].join(' ')}>
              {counts[key]}
            </span>
          )}
        </button>
      ))}
    </div>
  )
}
