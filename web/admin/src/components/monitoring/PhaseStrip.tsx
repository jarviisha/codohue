import { Stack, StatusDot } from '@astryxdesign/core'
import type { PhaseStatus } from '@/services/batchRuns'

const PHASE_NAMES: Array<'sparse' | 'dense' | 'trending'> = ['sparse', 'dense', 'trending']

type DotVariant = 'success' | 'error' | 'neutral'

type PhaseStripProps = {
  /**
   * phase_status from the wire. Defensive against undefined / null / wrong-
   * length values — older backends predating the BatchRunSummary shape may
   * omit the field entirely; rendering three "not run" cells is preferable
   * to crashing the parent page.
   */
  phaseStatus: PhaseStatus[] | null | undefined
  /**
   * Optional skipped reasons (per phase). When phase_status[i] is null and a
   * skipped reason is provided here, the dot tooltip explains the skip
   * (e.g. "dense_strategy=byoe"). For BatchRunSummary rows (no skipped
   * reason from the wire) leave this undefined.
   */
  skippedReasons?: Array<string | null>
}

const VARIANT_BY_STATUS: Record<Exclude<PhaseStatus, null>, { variant: DotVariant; label: string }> = {
  ok: { variant: 'success', label: 'ok' },
  fail: { variant: 'error', label: 'fail' },
  skipped: { variant: 'neutral', label: 'skipped' },
}

/**
 * PhaseStrip renders the three cron phases (sparse / dense / trending) as
 * three StatusDots side-by-side. Null status (phase did not run, e.g.
 * cancelled before reaching it) renders as a neutral "not run" dot so the
 * strip always shows exactly three slots and aligns across rows in a table.
 * Each dot carries a tooltip + aria-label naming the phase and its outcome,
 * so the strip stays readable despite being colour-led.
 */
export default function PhaseStrip({ phaseStatus, skippedReasons }: PhaseStripProps) {
  // Normalize the wire value: anything that isn't a real 3-slot array is
  // treated as "no phase data" and renders three placeholder cells.
  const phases: PhaseStatus[] = Array.isArray(phaseStatus) ? phaseStatus : []

  return (
    <Stack direction="horizontal" gap={2} align="center">
      {PHASE_NAMES.map((name, idx) => {
        const status = phases[idx] ?? null
        const cfg =
          status == null
            ? { variant: 'neutral' as DotVariant, label: 'not run' }
            : (VARIANT_BY_STATUS[status as Exclude<PhaseStatus, null>] ?? {
                variant: 'neutral' as DotVariant,
                label: String(status),
              })
        const tip =
          status === 'skipped' && skippedReasons?.[idx]
            ? `${name}: skipped (${skippedReasons[idx]})`
            : `${name}: ${cfg.label}`
        return <StatusDot key={name} variant={cfg.variant} label={tip} tooltip={tip} />
      })}
    </Stack>
  )
}
