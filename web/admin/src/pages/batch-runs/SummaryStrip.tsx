import { MetricTile } from '../../components/ui'
import type { BatchRunLog } from '../../types'
import { formatCount } from '../../utils/format'
import { fmtDateShort } from './format'

export default function SummaryStrip({ runs, total }: { runs: BatchRunLog[]; total: number }) {
  const completed = runs.filter(r => r.completed_at)
  const failed = completed.filter(r => !r.success)
  const running = runs.filter(r => !r.completed_at)
  const failRate = completed.length > 0 ? Math.round((failed.length / completed.length) * 100) : null
  const lastFail = failed[0]

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <MetricTile label="Total runs" value={formatCount(total)} valueClassName="text-lg" />
      <MetricTile
        label={`Fail rate (last ${completed.length})`}
        value={failRate != null ? `${failRate}%` : '—'}
        valueClassName={`text-lg ${failRate != null && failRate > 0 ? 'text-danger' : ''}`}
      />
      <MetricTile
        label="Currently running"
        value={running.length > 0 ? String(running.length) : '0'}
        sub={running.length > 0 ? 'live' : undefined}
        subClassName="text-accent"
        valueClassName="text-lg"
      />
      <MetricTile
        label="Last failure"
        value={lastFail ? `#${lastFail.id} · ${lastFail.namespace}` : 'None'}
        sub={lastFail ? fmtDateShort(lastFail.started_at) : undefined}
        valueClassName={`text-sm ${lastFail ? 'text-danger' : ''}`}
      />
    </div>
  )
}
