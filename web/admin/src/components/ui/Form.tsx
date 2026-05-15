import type { FormHTMLAttributes, ReactNode } from 'react'

interface FormProps extends FormHTMLAttributes<HTMLFormElement> {
  children: ReactNode
}

// Vertical form with consistent gap between fields.
export default function Form({ children, className = '', ...rest }: FormProps) {
  return (
    <form className={`flex flex-col gap-3 ${className}`} {...rest}>
      {children}
    </form>
  )
}
