import { Link } from 'react-router-dom'
import { useTriggerBatch } from '../../hooks/useTriggerBatch'
import { Button, CodeBadge, Modal } from '../../components/ui'
import type { BatchRunLog } from '../../types'
import { fmtDate } from './format'
import LogViewer from './LogViewer'
import PhaseBreakdown from './PhaseBreakdown'
import { RunStatus, TriggerBadge } from './badges'

export default function BatchRunModal({ run, onClose }: { run: BatchRunLog; onClose: () => void }) {
  const phase2Skipped = run.phase2_ok == null && run.phase1_ok != null
  const phase3Skipped = run.phase3_ok == null && run.phase1_ok != null
  const retrigger = useTriggerBatch(run.namespace)

  async function handleRetrigger() {
    try {
      await retrigger.mutateAsync()
      onClose()
    } catch {
      // error left visible via retrigger.isError
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={
        <span>
          Batch Run <span className="text-accent">#{run.id}</span>
        </span>
      }
    >
      <div className="flex flex-col gap-5">
        <div className="grid grid-cols-1 gap-x-6 gap-y-2 text-xs sm:grid-cols-2">
          <div className="text-muted">Namespace</div>
          <div><CodeBadge>{run.namespace}</CodeBadge></div>

          <div className="text-muted">Trigger</div>
          <div><TriggerBadge source={run.trigger_source} /></div>

          <div className="text-muted">Started</div>
          <div className="text-secondary tabular-nums">{fmtDate(run.started_at)}</div>

          <div className="text-muted">Completed</div>
          <div className="text-secondary tabular-nums">
            {run.completed_at
              ? fmtDate(run.completed_at)
              : <em className="not-italic text-accent">in progress</em>}
          </div>

          <div className="text-muted">Duration</div>
          <div className="text-secondary tabular-nums">
            {run.duration_ms != null
              ? `${run.duration_ms} ms`
              : run.completed_at
                ? '–'
                : <em className="not-italic text-accent">in progress</em>}
          </div>

          <div className="text-muted">Subjects processed</div>
          <div className="text-secondary tabular-nums">{run.subjects_processed ?? '—'}</div>

          <div className="text-muted">Status</div>
          <div><RunStatus run={run} /></div>

          {!run.success && run.completed_at && run.error_message && (
            <>
              <div className="text-muted">Error</div>
              <pre className="whitespace-pre-wrap text-danger text-[11px] m-0">{run.error_message}</pre>
            </>
          )}
        </div>

        {(run.phase1_ok != null || run.phase2_ok != null || run.phase3_ok != null) && (
          <div>
            <p className="m-0 mb-2 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">Phase breakdown</p>
            <div className="overflow-hidden rounded border border-default bg-surface py-2">
              <PhaseBreakdown run={run} phase2Skipped={phase2Skipped} phase3Skipped={phase3Skipped} />
            </div>
          </div>
        )}

        <div>
          <p className="m-0 mb-2 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
            Run log
            <span className="ml-2 normal-case font-normal text-muted">({run.log_lines.length} entries)</span>
          </p>
          <div className="overflow-hidden rounded border border-default bg-surface">
            <LogViewer entries={run.log_lines} />
          </div>
        </div>

        <div className="flex items-center justify-between gap-3 pt-1 border-t border-default">
          <Link
            to={`/namespaces/${encodeURIComponent(run.namespace)}/settings`}
            onClick={onClose}
            className="no-underline text-xs font-medium text-accent hover:text-accent-hover transition-colors"
          >
            View vector stats for {run.namespace}
          </Link>
          <div className="flex items-center gap-2">
            {retrigger.isError && (
              <span className="text-xs text-danger">Failed to trigger</span>
            )}
            <Button
              size="sm"
              onClick={handleRetrigger}
              disabled={retrigger.isPending}
            >
              {retrigger.isPending ? 'Running...' : 'Re-run'}
            </Button>
          </div>
        </div>
      </div>
    </Modal>
  )
}
