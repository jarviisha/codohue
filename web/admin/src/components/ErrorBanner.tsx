interface Props {
  message: string
  onDismiss?: () => void
}

export default function ErrorBanner({ message, onDismiss }: Props) {
  return (
    <div
      role="alert"
      className="flex justify-between items-center px-4 py-3 mb-4 rounded"
      style={{
        background: 'rgba(234,34,97,0.06)',
        border: '1px solid rgba(234,34,97,0.2)',
        borderRadius: '4px',
      }}
    >
      <span className="text-sm" style={{ color: '#ea2261' }}>{message}</span>
      {onDismiss && (
        <button
          onClick={onDismiss}
          aria-label="Dismiss error"
          className="bg-transparent border-0 cursor-pointer text-xl leading-none ml-4"
          style={{ color: '#ea2261' }}
        >
          ×
        </button>
      )}
    </div>
  )
}
