import type { ReactNode, SelectHTMLAttributes } from 'react'
import { fieldBaseClass, fieldInvalidClass } from './formClasses'

export interface DropdownOption {
  label: ReactNode
  value: string | number
  disabled?: boolean
}

export interface DropdownProps extends SelectHTMLAttributes<HTMLSelectElement> {
  controlSize?: 'sm' | 'md'
  error?: ReactNode
  helperText?: ReactNode
  options?: DropdownOption[]
  placeholder?: string
}

const sizeClasses: Record<NonNullable<DropdownProps['controlSize']>, string> = {
  sm: 'h-7 px-2 pr-7 text-xs',
  md: '',
}

export default function Dropdown({
  className = '',
  children,
  controlSize = 'md',
  error,
  helperText,
  options,
  placeholder,
  style,
  'aria-invalid': ariaInvalid,
  'aria-describedby': ariaDescribedBy,
  id,
  ...props
}: DropdownProps) {
  const helperId = helperText && id ? `${id}-helper` : undefined
  const errorId = error && id ? `${id}-error` : undefined
  const describedBy = [
    ariaDescribedBy,
    helperId,
    errorId,
  ].filter(Boolean).join(' ') || undefined

  return (
    <div className="flex flex-col gap-1">
      <select
        id={id}
        className={[
          fieldBaseClass,
          'appearance-none bg-no-repeat pr-8',
          sizeClasses[controlSize],
          error ? fieldInvalidClass : '',
          className,
        ].filter(Boolean).join(' ')}
        style={{
          backgroundImage: 'linear-gradient(45deg, transparent 50%, currentColor 50%), linear-gradient(135deg, currentColor 50%, transparent 50%)',
          backgroundPosition: 'calc(100% - 14px) 50%, calc(100% - 10px) 50%',
          backgroundSize: '4px 4px, 4px 4px',
          ...style,
        }}
        aria-invalid={ariaInvalid ?? (error ? true : undefined)}
        aria-describedby={describedBy}
        {...props}
      >
        {placeholder && <option value="">{placeholder}</option>}
        {options?.map(option => (
          <option key={String(option.value)} value={option.value} disabled={option.disabled}>
            {option.label}
          </option>
        ))}
        {children}
      </select>
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
