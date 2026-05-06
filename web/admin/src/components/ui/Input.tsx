import type { InputHTMLAttributes, ReactNode } from 'react'
import { fieldBaseClass, fieldInvalidClass } from './formClasses'

type InputSize = 'sm' | 'md' | 'compact'

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  error?: ReactNode
  helperText?: ReactNode
  inputSize?: InputSize
}

const sizeClasses: Record<InputSize, string> = {
  sm: 'h-8 px-2',
  md: '',
  compact: 'w-24 tabular-nums',
}

function fieldClassName({
  className = '',
  hasError,
  inputSize,
}: {
  className?: string
  hasError: boolean
  inputSize: InputSize
}) {
  return [
    fieldBaseClass,
    sizeClasses[inputSize],
    hasError ? fieldInvalidClass : '',
    className,
  ].filter(Boolean).join(' ')
}

export default function Input({
  className = '',
  error,
  helperText,
  inputSize = 'md',
  'aria-invalid': ariaInvalid,
  'aria-describedby': ariaDescribedBy,
  id,
  ...props
}: InputProps) {
  const helperId = helperText && id ? `${id}-helper` : undefined
  const errorId = error && id ? `${id}-error` : undefined
  const describedBy = [
    ariaDescribedBy,
    helperId,
    errorId,
  ].filter(Boolean).join(' ') || undefined

  return (
    <div className="flex flex-col gap-1">
      <input
        id={id}
        className={fieldClassName({ className, hasError: Boolean(error), inputSize })}
        aria-invalid={ariaInvalid ?? (error ? true : undefined)}
        aria-describedby={describedBy}
        {...props}
      />
      {helperText && (
        <p id={helperId} className="m-0 text-xs text-muted">
          {helperText}
        </p>
      )}
      {error && (
        <p id={errorId} className="m-0 text-xs font-medium text-danger">
          {error}
        </p>
      )}
    </div>
  )
}
