import type { ButtonHTMLAttributes, ReactNode } from 'react'

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
type ButtonSize = 'sm' | 'md' | 'icon'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  children?: ReactNode
}

const variantClasses: Record<ButtonVariant, string> = {
  primary: 'bg-accent hover:bg-accent-hover active:bg-accent-active text-accent-text border border-transparent',
  secondary: 'bg-transparent border border-default hover:border-strong hover:bg-surface-raised text-primary',
  ghost: 'bg-transparent border border-transparent text-secondary hover:bg-surface-raised hover:text-primary',
  danger: 'bg-danger-bg border border-danger/25 text-danger hover:bg-danger-bg hover:border-danger/40',
}

const sizeClasses: Record<ButtonSize, string> = {
  sm: 'h-8 px-2 py-0 text-sm leading-5',
  md: 'h-9 px-4 py-0 text-sm leading-5',
  icon: 'size-8 p-0 inline-flex items-center justify-center',
}

export default function Button({
  variant = 'secondary',
  size = 'md',
  className = '',
  children,
  ...props
}: ButtonProps) {
  return (
    <button
      className={`${variantClasses[variant]} ${sizeClasses[size]} inline-flex items-center justify-center font-medium rounded cursor-pointer transition-colors duration-150 disabled:opacity-60 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:shadow-focus ${className}`}
      {...props}
    >
      {children}
    </button>
  )
}
