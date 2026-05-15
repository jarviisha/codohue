import type { ReactNode } from 'react'
import Switch from './Switch'

interface ToggleRowProps {
  title: ReactNode
  description?: ReactNode
  checked: boolean
  onChange: (next: boolean) => void
  disabled?: boolean
  ariaLabel: string
}

// Title + description on the left, Switch on the right. Pairs with Field/Form
// when a setting deserves its own row instead of an inline Checkbox.
export default function ToggleRow({
  title,
  description,
  checked,
  onChange,
  disabled,
  ariaLabel,
}: ToggleRowProps) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-sm border border-default bg-surface-raised px-3 py-2">
      <div>
        <p className="text-sm font-semibold text-primary">{title}</p>
        {description ? <p className="text-sm text-muted">{description}</p> : null}
      </div>
      <Switch
        checked={checked}
        onChange={onChange}
        disabled={disabled}
        ariaLabel={ariaLabel}
      />
    </div>
  )
}
