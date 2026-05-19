import {
  Button,
  CodeBlock,
  KeyValueList,
  Modal,
  Notice,
  Panel,
  StatusToken,
} from '@/components/ui'
import type { KeyValueRow } from '@/components/ui'
import type { BatchRunLog, BatchRunLogEntry } from '@/services/batchRuns'
import { formatNumber, formatTimestamp } from '@/utils/format'
import {
  formatPhaseDuration,
  phaseToken,
  runStatusLabel,
  runStatusToken,
} from './helpers'

interface DetailModalProps {
  run: BatchRunLog | null
  onClose: () => void
}

interface PhaseSpec {
  label: string
  ok: boolean | null | undefined
  durationMs: number | null | undefined
  error: string | null | undefined
  counters: KeyValueRow[]
}

function phaseSpecs(run: BatchRunLog): PhaseSpec[] {
  return [
    {
      label: 'phase 1 · sparse',
      ok: run.phase1_ok,
      durationMs: run.phase1_duration_ms,
      error: run.phase1_error,
      counters: [
        { label: 'subjects', value: formatNumber(run.phase1_subjects) },
        { label: 'objects', value: formatNumber(run.phase1_objects) },
      ],
    },
    {
      label: 'phase 2 · dense',
      ok: run.phase2_ok,
      durationMs: run.phase2_duration_ms,
      error: run.phase2_error,
      counters: [
        { label: 'items', value: formatNumber(run.phase2_items) },
        { label: 'subjects', value: formatNumber(run.phase2_subjects) },
      ],
    },
    {
      label: 'phase 3 · trending',
      ok: run.phase3_ok,
      durationMs: run.phase3_duration_ms,
      error: run.phase3_error,
      counters: [{ label: 'items', value: formatNumber(run.phase3_items) }],
    },
  ]
}

function formatLogLines(lines: BatchRunLogEntry[]): string {
  return lines
    .map((line) => {
      const ts = line.ts ? `[${line.ts}] ` : ''
      const level = line.level ? `${line.level.toUpperCase()} ` : ''
      return `${ts}${level}${line.msg}`
    })
    .join('\n')
}

// Read-only inspector for a single batch run. The row already carries the full
// detail (per-phase counters + slog log_lines), so we don't refetch — just
// render whatever the list response handed us.
export default function BatchRunDetailModal({ run, onClose }: DetailModalProps) {
  if (!run) return null

  const isReEmbed = Boolean(run.target_strategy_id) || run.trigger_source === 'admin_reembed'
  const phases = phaseSpecs(run)
  const visiblePhases = isReEmbed
    ? []
    : phases.filter((p) => p.ok !== null && p.ok !== undefined)
  const targetStrategy =
    run.target_strategy_id && run.target_strategy_version
      ? `${run.target_strategy_id}@${run.target_strategy_version}`
      : run.target_strategy_id ?? null

  const summary: KeyValueRow[] = [
    {
      label: 'status',
      value: (
        <StatusToken
          state={runStatusToken(run)}
          label={runStatusLabel(run)}
          title={run.error_message ?? undefined}
        />
      ),
    },
    { label: 'id', value: `#${run.id}` },
    { label: 'namespace', value: run.namespace },
    { label: 'trigger', value: run.trigger_source },
    { label: 'started_at', value: formatTimestamp(run.started_at) },
    { label: 'completed_at', value: formatTimestamp(run.completed_at) },
    { label: 'duration', value: formatPhaseDuration(run.duration_ms) },
    { label: 'entities_processed', value: formatNumber(run.entities_processed) },
  ]
  if (targetStrategy) {
    summary.push({ label: 'target_strategy', value: targetStrategy })
  }

  const logLines = run.log_lines ?? []

  return (
    <Modal
      open
      onClose={onClose}
      size="lg"
      title={`batch run #${run.id}`}
      footer={
        <Button variant="ghost" onClick={onClose}>
          Close
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        {run.error_message ? (
          <Notice tone="fail" title="Run failed">
            <span className="font-mono">{run.error_message}</span>
          </Notice>
        ) : null}

        <Panel title="summary">
          <KeyValueList rows={summary} />
        </Panel>

        {visiblePhases.length > 0 ? (
          <Panel title="phases">
            <div className="flex flex-col gap-4">
              {visiblePhases.map((phase) => (
                <div key={phase.label} className="flex flex-col gap-2">
                  <div className="flex items-center gap-2">
                    <StatusToken
                      state={phaseToken(phase.ok)}
                      label={formatPhaseDuration(phase.durationMs)}
                    />
                    <span className="font-mono text-xs uppercase tracking-[0.04em] text-secondary">
                      {phase.label}
                    </span>
                  </div>
                  <KeyValueList rows={phase.counters} />
                  {phase.error ? (
                    <Notice tone="fail" title="Phase error">
                      <span className="font-mono">{phase.error}</span>
                    </Notice>
                  ) : null}
                </div>
              ))}
            </div>
          </Panel>
        ) : null}

        <Panel title={`log_lines (${logLines.length})`}>
          {logLines.length === 0 ? (
            <p className="text-sm text-muted">No captured log output.</p>
          ) : (
            <CodeBlock language="log" copyable maxHeight="24rem">
              {formatLogLines(logLines)}
            </CodeBlock>
          )}
        </Panel>
      </div>
    </Modal>
  )
}
