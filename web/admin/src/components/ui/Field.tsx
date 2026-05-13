import type { ReactNode } from 'react'

interface FieldProps {
  label: ReactNode
  hint?: ReactNode
  error?: ReactNode
  required?: boolean
  htmlFor?: string
  children: ReactNode
}

// Form-field wrapper. Label on top, control below, optional hint/error.
// For inline label/value rows in dense settings, use KeyValueList instead.
export default function Field({ label, hint, error, required, htmlFor, children }: FieldProps) {
  return (
    <div className="flex flex-col gap-1">
      <label
        htmlFor={htmlFor}
        className="text-sm text-secondary flex items-center gap-1"
      >
        <span>{label}</span>
        {required ? <span className="text-danger" aria-hidden>*</span> : null}
      </label>
      {children}
      {error ? (
        <p className="text-xs text-danger font-mono">{error}</p>
      ) : hint ? (
        <p className="text-xs text-muted">{hint}</p>
      ) : null}
    </div>
  )
}
