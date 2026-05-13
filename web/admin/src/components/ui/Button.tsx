import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react'

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
type ButtonSize = 'sm' | 'md' | 'lg'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  loading?: boolean
  leadingIcon?: ReactNode
}

const VARIANT: Record<ButtonVariant, string> = {
  // Primary uses accent-emphasis (the darker accent variant) so white text
  // hits WCAG AA-normal on dark mode (4.63:1) — `bg-accent` itself is the
  // text-only variant and would fail contrast as a button background.
  primary:
    'bg-accent-emphasis text-accent-text hover:opacity-90 border border-transparent disabled:opacity-50',
  secondary:
    'bg-surface text-primary border border-default hover:border-strong hover:bg-surface-raised disabled:opacity-50',
  ghost:
    'bg-transparent text-secondary border border-transparent hover:bg-surface-raised hover:text-primary disabled:opacity-50',
  danger:
    'bg-danger text-white hover:opacity-90 border border-transparent disabled:opacity-50',
}

const SIZE: Record<ButtonSize, string> = {
  sm: 'h-7 px-2 text-xs',
  md: 'h-8 px-3 text-sm',
  lg: 'h-9 px-4 text-sm',
}

const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', loading, leadingIcon, children, className = '', disabled, type = 'button', ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      disabled={disabled || loading}
      className={[
        'inline-flex items-center justify-center gap-1.5 rounded-sm font-sans transition-colors duration-100 focus:outline-none focus:shadow-focus',
        VARIANT[variant],
        SIZE[size],
        className,
      ].join(' ')}
      {...rest}
    >
      {loading ? (
        <span className="inline-block h-3 w-3 border-2 border-current border-t-transparent rounded-full animate-spin" aria-hidden />
      ) : leadingIcon ? (
        <span className="inline-flex" aria-hidden>{leadingIcon}</span>
      ) : null}
      <span>{children}</span>
    </button>
  )
})

export default Button
