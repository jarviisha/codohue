interface Props {
  message: string
  onDismiss?: () => void
}

export default function ErrorBanner({ message, onDismiss }: Props) {
  return (
    <div
      role="alert"
      className="flex justify-between items-center px-4 py-3 mb-4 rounded-xl bg-danger-bg border border-danger/25"
    >
      <span className="text-sm font-medium text-danger">{message}</span>
      {onDismiss && (
        <button
          onClick={onDismiss}
          aria-label="Dismiss error"
          className="bg-transparent border-0 cursor-pointer text-xl leading-none ml-4 text-danger hover:text-danger/70 transition-colors"
        >
          ×
        </button>
      )}
    </div>
  )
}
