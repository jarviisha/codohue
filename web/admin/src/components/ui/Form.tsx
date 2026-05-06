import type {
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
} from 'react'
import { inputClass } from './formClasses'

interface FormControlProps {
  label: ReactNode
  children: ReactNode
  htmlFor?: string
  inline?: boolean
  className?: string
  labelClassName?: string
}

export function FormControl({
  label,
  children,
  htmlFor,
  inline = false,
  className = '',
  labelClassName = '',
}: FormControlProps) {
  return (
    <div className={[
      inline ? 'flex items-center gap-4' : 'flex flex-col gap-1.5',
      className,
    ].filter(Boolean).join(' ')}
    >
      <label
        htmlFor={htmlFor}
        className={[
          inline ? 'min-w-48' : '',
          'text-[13px] font-medium text-primary',
          labelClassName,
        ].filter(Boolean).join(' ')}
      >
        {label}
      </label>
      {children}
    </div>
  )
}

export function FormSection({
  title,
  children,
  className = '',
}: {
  title: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={`flex flex-col gap-3 ${className}`}>
      <h3 className="m-0 border-b border-default pb-2 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
        {title}
      </h3>
      {children}
    </section>
  )
}

export function TextInput({
  className = '',
  ...props
}: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`${inputClass} ${className}`}
      {...props}
    />
  )
}

export function NumberInput({
  className = '',
  ...props
}: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      type="number"
      className={`${inputClass} w-24 tabular-nums ${className}`}
      {...props}
    />
  )
}

export function Select({
  className = '',
  children,
  ...props
}: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={`${inputClass} ${className}`}
      {...props}
    >
      {children}
    </select>
  )
}
