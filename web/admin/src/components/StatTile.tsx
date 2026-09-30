import { Link } from 'react-router-dom'
import { Card, Stack, StatusDot } from '@astryxdesign/core'

type StatTileTone = 'success' | 'warning' | 'error' | 'neutral'

/**
 * StatTile is the shared "label / big number / optional status dot" card
 * used on overview pages. The dot renders only when `hint` carries real
 * human-readable status text — the tone name itself is never shown (#51).
 * Numeric values are locale-formatted; pass a string to opt out.
 *
 * `description` is always-visible helper text (not a tooltip) so it reads the
 * same on touch, keyboard and screen readers. `href` turns the tile into a
 * link to the matching filtered list.
 */
export default function StatTile({
  label,
  value,
  tone = 'neutral',
  hint,
  description,
  href,
}: {
  label: string
  value: string | number
  tone?: StatTileTone
  hint?: string
  description?: string
  href?: string
}) {
  const formatted = typeof value === 'number' ? value.toLocaleString() : value
  const card = (
    <Card className="h-full">
      <Stack gap={6}>
        <span className="text-secondary text-xs uppercase tracking-wide">{label}</span>
        <Stack gap={4} direction="horizontal" align="center">
          <span className="text-primary text-xl font-semibold tabular-nums">{formatted}</span>
          {hint && (
            <Stack gap={2} direction="horizontal" align="center">
              <StatusDot variant={tone} label={hint} />
              <span aria-hidden="true" className="text-secondary text-xs">
                {hint}
              </span>
            </Stack>
          )}
        </Stack>
        {description && <span className="text-secondary text-xs">{description}</span>}
      </Stack>
    </Card>
  )
  if (!href) return <div className="flex-1 min-w-36">{card}</div>
  return (
    <Link to={href} className="flex-1 min-w-36 hover:opacity-80">
      {card}
      <span className="sr-only">View items</span>
    </Link>
  )
}
