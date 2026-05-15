import type { IconName } from './icons'

interface Props {
  name: IconName
  size?: number
  className?: string
}

export default function Icon({ name, size = 16, className = '' }: Props) {
  return (
    <svg width={size} height={size} aria-hidden="true" className={className}>
      <use href={`/icons.svg#${name}`} />
    </svg>
  )
}

export type { IconName }