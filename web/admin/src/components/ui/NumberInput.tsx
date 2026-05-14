import {
  forwardRef,
  useImperativeHandle,
  useRef,
  type InputHTMLAttributes,
} from 'react'

interface NumberInputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size' | 'type'> {
  invalid?: boolean
  width?: string // tailwind width class override (e.g. 'w-20'); defaults to 'w-24'
}

const NumberInput = forwardRef<HTMLInputElement, NumberInputProps>(function NumberInput(
  {
    invalid,
    width = 'w-24',
    className = '',
    disabled,
    readOnly,
    inputMode = 'decimal',
    ...rest
  },
  ref,
) {
  const inputRef = useRef<HTMLInputElement>(null)
  useImperativeHandle(ref, () => inputRef.current as HTMLInputElement)

  const step = (direction: -1 | 1) => {
    const input = inputRef.current
    if (!input || disabled || readOnly) return

    if (direction > 0) input.stepUp()
    else input.stepDown()

    input.dispatchEvent(new Event('input', { bubbles: true }))
  }

  return (
    <div
      className={[
        'inline-flex h-9 overflow-hidden rounded-sm border bg-surface',
        'focus-within:shadow-focus',
        invalid
          ? 'border-danger'
          : 'border-default hover:border-strong focus-within:border-accent',
        disabled ? 'opacity-50' : '',
        width,
      ].join(' ')}
    >
      <button
        type="button"
        aria-label="Decrease value"
        disabled={disabled || readOnly}
        onClick={() => step(-1)}
        className="flex h-full w-7 shrink-0 items-center justify-center border-r border-default bg-surface text-secondary hover:bg-surface-raised hover:text-primary disabled:hover:bg-surface disabled:hover:text-secondary"
      >
        <span className="font-mono text-sm leading-none">-</span>
      </button>
      <input
        ref={inputRef}
        type="number"
        inputMode={inputMode}
        disabled={disabled}
        readOnly={readOnly}
        className={[
          'number-input h-full min-w-0 flex-1 bg-transparent px-2 text-sm',
          'font-mono tabular-nums text-primary text-right',
          'placeholder:text-muted',
          'focus:outline-none',
          className,
        ].join(' ')}
        {...rest}
      />
      <button
        type="button"
        aria-label="Increase value"
        disabled={disabled || readOnly}
        onClick={() => step(1)}
        className="flex h-full w-7 shrink-0 items-center justify-center border-l border-default bg-surface text-secondary hover:bg-surface-raised hover:text-primary disabled:hover:bg-surface disabled:hover:text-secondary"
      >
        <span className="font-mono text-sm leading-none">+</span>
      </button>
    </div>
  )
})

export default NumberInput
