interface LoadingStateProps {
  label?: string
  className?: string
}

export default function LoadingState({ label = 'Loading...', className = '' }: LoadingStateProps) {
  return (
    <div className={`text-sm text-muted ${className}`} role="status" aria-live="polite">
      {label}
    </div>
  )
}
