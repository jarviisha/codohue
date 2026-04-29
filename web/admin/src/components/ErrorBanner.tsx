interface Props {
  message: string
  onDismiss?: () => void
}

export default function ErrorBanner({ message, onDismiss }: Props) {
  return (
    <div role="alert" style={{ background: '#ffeaea', color: '#c00', padding: '0.75rem 1rem', borderRadius: 4, marginBottom: '1rem', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
      <span>{message}</span>
      {onDismiss && (
        <button onClick={onDismiss} aria-label="Dismiss error" style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#c00', fontSize: '1.2rem', lineHeight: 1 }}>
          ×
        </button>
      )}
    </div>
  )
}
