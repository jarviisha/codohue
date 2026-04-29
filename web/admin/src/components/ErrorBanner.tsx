interface Props {
  message: string
  onDismiss?: () => void
}

export default function ErrorBanner({ message, onDismiss }: Props) {
  return (
    <div role="alert" className="bg-red-50 text-red-700 py-3 px-4 rounded mb-4 flex justify-between items-center">
      <span>{message}</span>
      {onDismiss && (
        <button
          onClick={onDismiss}
          aria-label="Dismiss error"
          className="bg-transparent border-0 cursor-pointer text-red-700 text-xl leading-none ml-4"
        >
          ×
        </button>
      )}
    </div>
  )
}
