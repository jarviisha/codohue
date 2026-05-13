import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  type InputHTMLAttributes,
} from 'react'

interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'size'> {
  indeterminate?: boolean
}

// Native <input type="checkbox"> styled by accent-color (base CSS rule).
// Browser draws the check glyph; we only size + focus the box. The
// `indeterminate` prop is a DOM-only state set imperatively per React docs.
const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(function Checkbox(
  { indeterminate, className = '', ...rest },
  ref,
) {
  const innerRef = useRef<HTMLInputElement>(null)
  useImperativeHandle(ref, () => innerRef.current as HTMLInputElement, [])

  useEffect(() => {
    if (innerRef.current) innerRef.current.indeterminate = Boolean(indeterminate)
  }, [indeterminate])

  return (
    <input
      ref={innerRef}
      type="checkbox"
      className={[
        'h-4 w-4 rounded-sm border border-default bg-surface',
        'focus:outline-none focus:shadow-focus',
        'disabled:opacity-50',
        className,
      ].join(' ')}
      {...rest}
    />
  )
})

export default Checkbox
