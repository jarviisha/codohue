import { Card, Stack, StatusDot } from '@astryxdesign/core'

type StatTileTone = 'success' | 'warning' | 'error' | 'neutral'

/**
 * StatTile is the shared "label / big number / optional status dot" card
 * used on overview pages. The dot renders only when `hint` carries real
 * human-readable status text — the tone name itself is never shown (#51).
 * Numeric values are locale-formatted; pass a string to opt out.
 */
export default function StatTile({
  label,
  value,
  tone = 'neutral',
  hint,
}: {
  label: string
  value: string | number
  tone?: StatTileTone
  hint?: string
}) {
  return (
    <Card className="flex-1 min-w-36">
      <Stack gap={6}>
        <span className="text-secondary text-xs uppercase tracking-wide">{label}</span>
        <Stack gap={4} direction="horizontal" align="center">
          <span className="text-primary text-xl font-semibold tabular-nums">
            {typeof value === 'number' ? value.toLocaleString() : value}
          </span>
          {hint && (
            <Stack gap={2} direction="horizontal" align="center">
              <StatusDot variant={tone} label={hint} />
              <span aria-hidden="true" className="text-secondary text-xs">
                {hint}
              </span>
            </Stack>
          )}
        </Stack>
      </Stack>
    </Card>
  )
}
