import type { ReactNode } from 'react'

/**
 * MetaLine renders a row of secondary metadata facts (counts, timestamps,
 * config values) as separate spans laid out with whitespace instead of a
 * punctuation separator, so a subtitle reads as distinct facts without any
 * separator glyph between them.
 *
 * Falsy items are dropped, so callers can inline conditionals:
 *
 *   <MetaLine items={[`${total} matching`, page > 0 && `page ${page + 1}`]} />
 */
export default function MetaLine({
  items,
  size = 'sm',
  className = '',
}: {
  items: Array<ReactNode | false | null | undefined>
  size?: 'sm' | 'xs'
  className?: string
}) {
  const visible = items.filter(Boolean)
  if (visible.length === 0) return null
  return (
    <div
      className={`text-foreground-subtle flex flex-wrap items-center gap-x-5 gap-y-1 ${
        size === 'xs' ? 'text-xs' : 'text-sm'
      } ${className}`.trim()}
    >
      {visible.map((item, i) => (
        <span key={i}>{item}</span>
      ))}
    </div>
  )
}
