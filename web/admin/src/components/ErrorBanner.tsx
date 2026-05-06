import { Notice } from './ui'

interface Props {
  message: string
  onDismiss?: () => void
}

export default function ErrorBanner({ message, onDismiss }: Props) {
  return (
    <Notice tone="danger" role="alert" onDismiss={onDismiss} className="mb-4">
      {message}
    </Notice>
  )
}
