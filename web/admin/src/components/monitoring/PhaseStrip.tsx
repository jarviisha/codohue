import { Badge, Inline, Tooltip } from '@jarviisha/davinci-react-ui'
import type { PhaseStatus } from '@/services/batchRuns'

const PHASE_NAMES: Array<'sparse' | 'dense' | 'trending'> = ['sparse', 'dense', 'trending']

type PhaseTone = 'success' | 'danger' | 'neutral'

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
   * skipped reason is provided here, the badge tooltip explains the skip
   * (e.g. "dense_strategy=byoe"). For BatchRunSummary rows (no skipped
   * reason from the wire) leave this undefined.
   */
  skippedReasons?: Array<string | null>
}

const TONE_BY_STATUS: Record<Exclude<PhaseStatus, null>, { tone: PhaseTone; label: string }> = {
  ok: { tone: 'success', label: 'ok' },
  fail: { tone: 'danger', label: 'fail' },
  skipped: { tone: 'neutral', label: 'skip' },
}

/**
 * PhaseStrip renders the three cron phases (sparse / dense / trending) as
 * three Davinci Badges side-by-side. Null status (phase did not run, e.g.
 * cancelled before reaching it) renders as a "—" placeholder so the strip
 * always shows exactly three slots and aligns across rows in a table.
 */
export default function PhaseStrip({ phaseStatus, skippedReasons }: PhaseStripProps) {
  // Normalize the wire value: anything that isn't a real 3-slot array is
  // treated as "no phase data" and renders three placeholder cells.
  const phases: PhaseStatus[] = Array.isArray(phaseStatus) ? phaseStatus : []

  return (
    <Inline align="center">
      {PHASE_NAMES.map((name, idx) => {
        const status = phases[idx] ?? null
        if (status == null) {
          return (
            <Tooltip key={name} content={`${name}: not run`}>
              <Badge variant="neutral">—</Badge>
            </Tooltip>
          )
        }
        const cfg = TONE_BY_STATUS[status as Exclude<PhaseStatus, null>] ?? {
          tone: 'neutral' as PhaseTone,
          label: String(status),
        }
        const tip =
          status === 'skipped' && skippedReasons?.[idx]
            ? `${name}: skipped (${skippedReasons[idx]})`
            : `${name}: ${cfg.label}`
        return (
          <Tooltip key={name} content={tip}>
            <Badge variant={cfg.tone}>{cfg.label}</Badge>
          </Tooltip>
        )
      })}
    </Inline>
  )
}
