import { Link } from 'react-router-dom'
import type { ReactNode } from 'react'

type Size = 'sm' | 'md' | 'lg'
type Variant = 'solid' | 'outline' | 'ghost' | 'soft'
type Tone = 'primary' | 'neutral' | 'danger'

/**
 * LinkButton is a navigation link that looks like a Button.
 *
 * It exists because `<Link><Button /></Link>` nests an interactive element
 * inside another one: invalid HTML, and screen readers and keyboard users get
 * two focus stops with undefined activation semantics. The obvious fix —
 * `<Button onClick={navigate}>` — is valid but throws away what a link is:
 * middle-click, ctrl-click, "open in new tab", and the status-bar target
 * preview all disappear when navigation stops being an anchor.
 *
 * So the anchor stays the element and only the Button's classes are borrowed.
 * That couples this file to the design system's class names — the one place
 * in the app that does — which is the price of keeping anchors as anchors
 * until the design system offers a polymorphic Button.
 */
export default function LinkButton({
  to,
  children,
  size = 'md',
  variant = 'solid',
  tone = 'primary',
  className = '',
}: {
  to: string
  children: ReactNode
  size?: Size
  variant?: Variant
  tone?: Tone
  className?: string
}) {
  const classes = [
    'davinci-button',
    `davinci-button--${size}`,
    `davinci-button--${variant}`,
    `davinci-button--tone-${tone}`,
    className,
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <Link to={to} className={classes}>
      {children}
    </Link>
  )
}
