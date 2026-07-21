/**
 * NamespaceTag renders a namespace name consistently across the whole admin
 * UI: highlighted in the primary accent colour on a light primary background,
 * so a namespace always reads as the same recognisable token whether it sits
 * in a heading, a table cell, or a breadcrumb.
 *
 * It renders a bare inline <span>, so it composes inside <h1>, <Link>, and
 * plain text without imposing layout.
 */
export default function NamespaceTag({
  name,
  className = '',
}: {
  name: string
  className?: string
}) {
  return (
    <span
      className={`text-primary font-bold bg-primary/10 rounded px-1.5 py-0.5 ${className}`.trim()}
    >
      {name}
    </span>
  )
}
