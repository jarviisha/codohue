import { forwardRef, type InputHTMLAttributes } from 'react'

interface NumberInputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size' | 'type'> {
  invalid?: boolean
  width?: string // tailwind width class override (e.g. 'w-20'); defaults to 'w-24'
}

const NumberInput = forwardRef<HTMLInputElement, NumberInputProps>(function NumberInput(
  { invalid, width = 'w-24', className = '', ...rest },
  ref,
) {
  return (
    <input
      ref={ref}
      type="number"
      inputMode="numeric"
      className={[
        'h-9 px-3 text-sm rounded-sm border bg-surface',
        'font-mono tabular-nums text-primary text-right',
        'placeholder:text-muted',
        'focus:outline-none focus:shadow-focus',
        invalid ? 'border-danger' : 'border-default hover:border-strong focus:border-accent',
        width,
        className,
      ].join(' ')}
      {...rest}
    />
  )
})

export default NumberInput
