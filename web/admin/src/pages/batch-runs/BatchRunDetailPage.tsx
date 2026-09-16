import { useEffect, useMemo, useReducer } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Badge, Banner, Button, Skeleton, Stack } from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import {
  useBatchRunDetail,
  useCancelBatchRun,
  useRetryBatchRun,
  type BatchRunDetail,
  type LogLine,
  type PhaseEntry,
} from '@/services/batchRuns'
import { useServerStream } from '@/services/stream'
import PageHeader from '@/components/shell/PageHeader'
import PhaseStrip from '@/components/monitoring/PhaseStrip'
import LogLineViewer from '@/components/monitoring/LogLineViewer'
import MetaLine from '@/components/MetaLine'
import NamespaceTag from '@/components/NamespaceTag'

type LocalState = {
  log: LogLine[]
  phases: PhaseEntry[]
  completed: boolean
  cancelled: boolean
}

type Action =
  | { type: 'hydrate'; run: BatchRunDetail }
  | { type: 'log_line'; line: LogLine }
  | { type: 'phase_completed'; payload: { phase: number; ok: boolean; duration_ms: number; count1?: number; count2?: number; error?: string } }
  | { type: 'run_completed' }
  | { type: 'run_cancelled' }

/**
 * mergePhase replaces phases[n-1] with the SSE-delivered result, preserving
 * the slots that haven't been touched yet (so the strip stays length 3).
 */
function mergePhase(phases: PhaseEntry[], n: number, ok: boolean, dur: number, error?: string): PhaseEntry[] {
  return phases.map((p) => {
    if (p.n !== n) return p
    return { ...p, ok, duration_ms: dur, error: error ?? null }
  })
}

function reducer(state: LocalState, action: Action): LocalState {
  switch (action.type) {
    case 'hydrate':
      return {
        log: action.run.log_lines,
        phases: action.run.phases,
        completed: action.run.completed_at != null,
        cancelled: action.run.error_message === 'operator_cancelled',
      }
    case 'log_line':
      return { ...state, log: [...state.log, action.line] }
    case 'phase_completed':
      return {
        ...state,
        phases: mergePhase(
          state.phases,
          action.payload.phase,
          action.payload.ok,
          action.payload.duration_ms,
          action.payload.error,
        ),
      }
    case 'run_completed':
      return { ...state, completed: true }
    case 'run_cancelled':
      return { ...state, completed: true, cancelled: true }
  }
}

export default function BatchRunDetailPage() {
  const { id, ns } = useParams<{ id: string; ns?: string }>()
  const navigate = useNavigate()
  const numericID = id != null ? Number(id) : null
  const detail = useBatchRunDetail(numericID)
  const cancel = useCancelBatchRun()
  const retry = useRetryBatchRun()

  const [state, dispatch] = useReducer(reducer, {
    log: [],
    phases: [],
    completed: false,
    cancelled: false,
  })

  // Hydrate local reducer state from the REST snapshot every time the query
  // refreshes — that way the SSE deltas always stack on top of the latest
  // server view, and a manual refresh (e.g. after cancel) updates the page.
  useEffect(() => {
    if (detail.data) {
      dispatch({ type: 'hydrate', run: detail.data })
    }
  }, [detail.data])

  // Subscribe to the per-run SSE stream only when the run is still in flight.
  // The handler closes itself on `completed` / `cancelled`; we mirror the
  // state into the reducer so the UI flips the badge + actions immediately.
  const streamURL = useMemo(() => {
    if (numericID == null || state.completed) return null
    return `/api/admin/v1/batch-runs/${numericID}/stream`
  }, [numericID, state.completed])

  const streamHandlers = useMemo(
    () => ({
      log_line: (data: unknown) => {
        const line = data as LogLine
        dispatch({ type: 'log_line', line })
      },
      phase_completed: (data: unknown) => {
        dispatch({
          type: 'phase_completed',
          payload: data as { phase: number; ok: boolean; duration_ms: number; count1?: number; count2?: number; error?: string },
        })
      },
      completed: () => dispatch({ type: 'run_completed' }),
      cancelled: () => dispatch({ type: 'run_cancelled' }),
    }),
    [],
  )

  useServerStream(streamURL, streamHandlers, { enabled: streamURL != null })

  if (detail.isLoading) {
    return (
      <PageContainer size="full">
        <Skeleton className="h-48 w-full" />
      </PageContainer>
    )
  }
  if (detail.isError) {
    return (
      <PageContainer size="full">
        <Banner status="error" title="Failed to load run" description={detail.error?.message ?? ''} />
      </PageContainer>
    )
  }
  const run = detail.data!

  return (
    <PageContainer size="full">
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full" wrap="wrap">
          <Stack gap={1}>
            <Stack gap={4} direction="horizontal" align="center">
              <h1 className="text-primary text-xl font-semibold">Run #{run.id}</h1>
              <Badge variant={run.kind === 'reembed' ? 'purple' : 'neutral'} label={run.kind} />
              <RunStateBadge state={state} run={run} />
            </Stack>
            <MetaLine
              items={[
                <Link to={`/ns/${encodeURIComponent(run.namespace)}`}>
                  <NamespaceTag name={run.namespace} />
                </Link>,
                `trigger=${run.trigger_source}`,
                `started ${new Date(run.started_at).toLocaleString()}`,
              ]}
            />
          </Stack>
          <Stack gap={4} direction="horizontal" align="center">
            {!state.completed && (
              <Button
                
                variant="destructive"
                size="sm"
                onClick={() => numericID != null && cancel.mutate(numericID)}
                isDisabled={cancel.isPending} label={cancel.isPending ? 'Cancelling…' : 'Cancel'} />
            )}
            {state.completed && run.kind === 'cf' && (
              <Button
                size="sm"
                onClick={() => {
                  if (numericID == null) return
                  retry.mutate(numericID, {
                    onSuccess: (resp) => {
                      if (resp.id) {
                        navigate(
                          ns
                            ? `/ns/${encodeURIComponent(ns)}/batch-runs/${resp.id}`
                            : `/batch-runs/${resp.id}`,
                        )
                      }
                    },
                  })
                }}
                isDisabled={retry.isPending} label={retry.isPending ? 'Retrying…' : 'Retry'} />
            )}
          </Stack>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        {cancel.error && <Banner status="error" title="Cancel failed" description={cancel.error.message} />}
        {retry.error && <Banner status="error" title="Retry failed" description={retry.error.message} />}

        <Stack gap={6}>
          <Stack gap={4} direction="horizontal" align="center" justify="between">
            <h2 className="text-primary text-sm font-semibold">Phases</h2>
            <PhaseStrip phaseStatus={state.phases.map((p) => phaseToStatus(p))} />
          </Stack>
          <Stack gap={6}>
            {state.phases.map((p) => (
              <PhaseRow key={p.n} phase={p} />
            ))}
          </Stack>
        </Stack>

        <Stack gap={6}>
          <Stack gap={6}>
            <h2 className="text-primary text-sm font-semibold">Log</h2>
            <p className="text-secondary text-xs">
              {state.completed
                ? 'Final captured log lines.'
                : 'Streaming live — new lines arrive as cron emits them.'}
            </p>
          </Stack>
          <LogLineViewer lines={state.log} follow={!state.completed} />
        </Stack>
      </Stack>
    </PageContainer>
  )
}

function phaseToStatus(p: PhaseEntry): 'ok' | 'fail' | 'skipped' | null {
  if (p.ok === true) return 'ok'
  if (p.ok === false) return 'fail'
  if (p.skipped != null) return 'skipped'
  return null
}

function PhaseRow({ phase }: { phase: PhaseEntry }) {
  return (
    <Stack gap={4} direction="horizontal" align="center" justify="between">
      <Stack gap={4} direction="horizontal" align="center">
        <span className="text-secondary text-xs uppercase tracking-wide w-16">
          phase {phase.n}
        </span>
        <span className="text-primary font-medium">{phase.name}</span>
        {phase.ok === true && <Badge variant="success" label="ok" />}
        {phase.ok === false && <Badge variant="error" label="fail" />}
        {phase.ok === null && phase.skipped && <Badge variant="neutral" label="skipped" />}
      </Stack>
      <Stack gap={4} direction="horizontal" align="center">
        {phase.subjects != null && (
          <span className="text-secondary text-sm tabular-nums">
            subjects: <span className="text-primary">{phase.subjects.toLocaleString()}</span>
          </span>
        )}
        {phase.objects != null && (
          <span className="text-secondary text-sm tabular-nums">
            objects: <span className="text-primary">{phase.objects.toLocaleString()}</span>
          </span>
        )}
        {phase.items != null && (
          <span className="text-secondary text-sm tabular-nums">
            items: <span className="text-primary">{phase.items.toLocaleString()}</span>
          </span>
        )}
        <span className="text-secondary text-sm tabular-nums">
          {phase.duration_ms > 0 ? `${(phase.duration_ms / 1000).toFixed(1)}s` : '—'}
        </span>
      </Stack>
    </Stack>
  )
}

function RunStateBadge({ state, run }: { state: LocalState; run: BatchRunDetail }) {
  if (!state.completed) {
    return (
      <Badge variant={run.cancel_requested ? 'warning' : 'info'} label={run.cancel_requested ? 'cancelling' : 'running'} />
    )
  }
  if (state.cancelled) return <Badge variant="neutral" label="cancelled" />
  if (run.success) return <Badge variant="success" label="ok" />
  return <Badge variant="error" label="failed" />
}
