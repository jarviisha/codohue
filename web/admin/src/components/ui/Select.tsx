import { forwardRef, type SelectHTMLAttributes } from 'react'

type SelectSize = 'sm' | 'md'

interface SelectProps extends Omit<SelectHTMLAttributes<HTMLSelectElement>, 'size'> {
  selectSize?: SelectSize
  invalid?: boolean
}

const SIZE: Record<SelectSize, string> = {
  sm: 'h-7 px-2 text-xs',
  md: 'h-8 px-2.5 text-sm',
}

const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { selectSize = 'md', invalid, className = '', children, ...rest },
  ref,
) {
  return (
    <select
      ref={ref}
      className={[
        'rounded-sm border bg-surface text-primary appearance-none pr-7',
        'bg-no-repeat bg-[right_0.5rem_center]',
        'focus:outline-none focus:shadow-focus',
        invalid ? 'border-danger' : 'border-default hover:border-strong focus:border-accent',
        SIZE[selectSize],
        className,
      ].join(' ')}
      {...rest}
    >
      {children}
    </select>
  )
})

export default Select
