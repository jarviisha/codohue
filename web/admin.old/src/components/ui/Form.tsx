import type {
  ReactNode,
} from 'react'
import Dropdown, { type DropdownProps } from './Dropdown'
import Input, { type InputProps } from './Input'

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
      inline ? 'flex items-center gap-3' : 'flex flex-col gap-1',
      className,
    ].filter(Boolean).join(' ')}
    >
      <label
        htmlFor={htmlFor}
        className={[
          inline ? 'min-w-40' : '',
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
    <section className={`flex flex-col gap-2.5 ${className}`}>
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
}: InputProps) {
  return (
    <Input
      className={className}
      {...props}
    />
  )
}

export function NumberInput({
  className = '',
  ...props
}: InputProps) {
  return (
    <Input
      type="number"
      inputSize="compact"
      className={className}
      {...props}
    />
  )
}

export function Select({
  className = '',
  children,
  ...props
}: DropdownProps) {
  return (
    <Dropdown
      className={className}
      {...props}
    >
      {children}
    </Dropdown>
  )
}
