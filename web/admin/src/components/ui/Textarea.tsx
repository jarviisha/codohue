import { forwardRef, type TextareaHTMLAttributes } from 'react'

interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  invalid?: boolean
  mono?: boolean
}

const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { invalid, mono = false, className = '', ...rest },
  ref,
) {
  return (
    <textarea
      ref={ref}
      spellCheck={mono ? false : rest.spellCheck}
      className={[
        'min-h-24 rounded-sm border bg-surface p-3 text-sm text-primary placeholder:text-muted',
        'focus:outline-none focus:shadow-focus',
        mono ? 'font-mono' : '',
        invalid
          ? 'border-danger'
          : 'border-default hover:border-strong focus:border-accent',
        className,
      ].join(' ')}
      {...rest}
    />
  )
})

export default Textarea
