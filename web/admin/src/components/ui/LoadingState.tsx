interface LoadingStateProps {
  rows?: number // shimmer row count
  label?: string // mono header above the rows
}

// Shimmer skeleton placeholder. Use this instead of full-page spinners.
export default function LoadingState({ rows = 3, label = 'loading' }: LoadingStateProps) {
  return (
    <div className="flex flex-col gap-2" aria-busy="true" aria-live="polite">
      <p className="font-mono text-[11px] uppercase tracking-[0.12em] text-muted">{label}</p>
      {Array.from({ length: rows }).map((_, i) => (
        <div
          key={i}
          className="h-4 bg-surface-raised rounded-sm animate-pulse"
          style={{ width: `${80 - i * 10}%` }}
        />
      ))}
    </div>
  )
}
