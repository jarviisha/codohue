import { forwardRef, type InputHTMLAttributes } from 'react'

type InputSize = 'sm' | 'md'

interface InputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size'> {
  inputSize?: InputSize
  invalid?: boolean
}

const SIZE: Record<InputSize, string> = {
  sm: 'h-7 px-2 text-xs',
  md: 'h-8 px-2.5 text-sm',
}

const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { inputSize = 'md', invalid, className = '', type = 'text', ...rest },
  ref,
) {
  return (
    <input
      ref={ref}
      type={type}
      className={[
        'rounded-sm border bg-surface text-primary placeholder:text-muted',
        'focus:outline-none focus:shadow-focus',
        invalid ? 'border-danger' : 'border-default hover:border-strong focus:border-accent',
        SIZE[inputSize],
        className,
      ].join(' ')}
      {...rest}
    />
  )
})

export default Input
