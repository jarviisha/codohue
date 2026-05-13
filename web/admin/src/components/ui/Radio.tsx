import { forwardRef, type InputHTMLAttributes, type ReactNode } from 'react'

type RadioProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'size'>

// Native <input type="radio"> tinted by accent-color (base CSS rule).
const Radio = forwardRef<HTMLInputElement, RadioProps>(function Radio(
  { className = '', ...rest },
  ref,
) {
  return (
    <input
      ref={ref}
      type="radio"
      className={[
        'h-4 w-4 border border-default bg-surface',
        'focus:outline-none focus:shadow-focus',
        'disabled:opacity-50',
        className,
      ].join(' ')}
      {...rest}
    />
  )
})

export default Radio

export interface RadioOption<V extends string = string> {
  value: V
  label: ReactNode
  hint?: ReactNode
  disabled?: boolean
}

interface RadioGroupProps<V extends string = string> {
  name: string
  value: V | undefined
  onChange: (next: V) => void
  options: RadioOption<V>[]
  ariaLabel?: string
}

// Vertical radio group with one labelled row per option. Pairs with the
// shared `Field` primitive when used inside a form.
export function RadioGroup<V extends string = string>({
  name,
  value,
  onChange,
  options,
  ariaLabel,
}: RadioGroupProps<V>) {
  return (
    <div role="radiogroup" aria-label={ariaLabel} className="flex flex-col gap-2">
      {options.map((opt) => {
        const id = `${name}-${opt.value}`
        return (
          <label
            key={opt.value}
            htmlFor={id}
            className={[
              'flex items-start gap-2',
              opt.disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer',
            ].join(' ')}
          >
            <Radio
              id={id}
              name={name}
              value={opt.value}
              checked={value === opt.value}
              disabled={opt.disabled}
              onChange={() => onChange(opt.value)}
              className="mt-0.5"
            />
            <span className="flex flex-col">
              <span className="text-sm text-primary">{opt.label}</span>
              {opt.hint ? (
                <span className="text-xs text-muted">{opt.hint}</span>
              ) : null}
            </span>
          </label>
        )
      })}
    </div>
  )
}
