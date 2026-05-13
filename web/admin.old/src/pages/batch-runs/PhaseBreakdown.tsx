import { Badge, Table, Tbody, Td, Th, Thead, Tr } from '../../components/ui'
import type { BatchRunLog } from '../../types'

interface PhaseRowProps {
  label: string
  ok: boolean | null | undefined
  durMs: number | null | undefined
  counts: { label: string; value: number | null | undefined }[]
  error: string | null | undefined
  skipped?: boolean
}

function PhaseRow({ label, ok, durMs, counts, error, skipped }: PhaseRowProps) {
  const cell = 'py-1.5 text-[11px]'
  if (skipped) {
    return (
      <Tr>
        <Td className={`${cell} w-28`}>{label}</Td>
        <Td colSpan={3} muted className={`${cell} italic`}>skipped</Td>
      </Tr>
    )
  }
  if (ok == null) {
    return (
      <Tr>
        <Td className={`${cell} w-28`}>{label}</Td>
        <Td colSpan={3} muted className={`${cell} italic`}>no data</Td>
      </Tr>
    )
  }
  return (
    <Tr>
      <Td className={`${cell} w-28`}>{label}</Td>
      <Td className={cell}>
        {ok
          ? <Badge tone="success" dot>OK</Badge>
          : <Badge tone="danger" dot>Failed</Badge>}
      </Td>
      <Td mono muted className={cell}>
        {durMs != null ? `${durMs} ms` : '—'}
      </Td>
      <Td mono muted className={cell}>
        {counts.map(c => c.value != null ? `${c.label}: ${c.value}` : null).filter(Boolean).join('  ·  ')}
        {error && (
          <details className="mt-0.5">
            <summary className="cursor-pointer text-danger text-[11px]">error</summary>
            <pre className="mt-1 whitespace-pre-wrap text-danger text-[11px]">{error}</pre>
          </details>
        )}
      </Td>
    </Tr>
  )
}

export default function PhaseBreakdown({
  run,
  phase2Skipped,
  phase3Skipped,
}: {
  run: BatchRunLog
  phase2Skipped: boolean
  phase3Skipped: boolean
}) {
  return (
    <Table className="overflow-hidden">
      <Thead>
        <Th className="w-28">Phase</Th>
        <Th>Result</Th>
        <Th>Duration</Th>
        <Th>Counts</Th>
      </Thead>
      <Tbody>
        <PhaseRow
          label="1 · Sparse CF"
          ok={run.phase1_ok}
          durMs={run.phase1_duration_ms}
          counts={[
            { label: 'subjects', value: run.phase1_subjects },
            { label: 'objects', value: run.phase1_objects },
          ]}
          error={run.phase1_error}
        />
        <PhaseRow
          label="2 · Dense"
          ok={run.phase2_ok}
          durMs={run.phase2_duration_ms}
          counts={[
            { label: 'items', value: run.phase2_items },
            { label: 'subjects', value: run.phase2_subjects },
          ]}
          error={run.phase2_error}
          skipped={phase2Skipped}
        />
        <PhaseRow
          label="3 · Trending"
          ok={run.phase3_ok}
          durMs={run.phase3_duration_ms}
          counts={[{ label: 'items', value: run.phase3_items }]}
          error={run.phase3_error}
          skipped={phase3Skipped}
        />
      </Tbody>
    </Table>
  )
}
