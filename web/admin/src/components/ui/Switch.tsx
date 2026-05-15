interface SwitchProps {
  checked: boolean
  onChange: (next: boolean) => void
  disabled?: boolean
  ariaLabel: string
}

// Boolean toggle. `<button role="switch">` so screen readers announce state
// correctly. Track + knob are pure CSS (no SVG, no icon).
export default function Switch({ checked, onChange, disabled, ariaLabel }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={[
        'relative inline-flex h-5 w-9 items-center rounded-full border transition-colors duration-100',
        'focus:outline-none focus:shadow-focus',
        'disabled:opacity-50',
        checked
          ? 'bg-accent-emphasis border-accent-emphasis'
          : 'bg-surface-raised border-strong',
      ].join(' ')}
    >
      <span
        aria-hidden
        className={[
          'inline-block h-3.5 w-3.5 rounded-full bg-white shadow-raised transition-transform duration-100',
          checked ? 'translate-x-4' : 'translate-x-0.5',
        ].join(' ')}
      />
    </button>
  )
}
