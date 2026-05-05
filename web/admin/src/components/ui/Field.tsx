import type { ReactNode } from 'react'

interface FieldProps {
  label: string
  children: ReactNode
  inline?: boolean
  className?: string
  labelClassName?: string
}

export const inputClass = 'bg-surface border border-default hover:border-strong focus:border-accent focus:shadow-focus text-primary placeholder:text-muted text-sm px-3 py-2 rounded-lg focus:outline-none transition-shadow duration-100'

export default function Field({
  label,
  children,
  inline = false,
  className = '',
  labelClassName = '',
}: FieldProps) {
  return (
    <div className={`mb-3 ${inline ? 'flex items-center gap-4' : ''} ${className}`}>
      <label className={`text-[13px] font-medium text-primary ${inline ? 'min-w-48' : 'block mb-1.5'} ${labelClassName}`}>
        {label}
      </label>
      {children}
    </div>
  )
}
