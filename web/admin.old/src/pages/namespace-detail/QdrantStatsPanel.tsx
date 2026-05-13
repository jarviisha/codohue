import { MetricTile, Panel } from '../../components/ui'
import type { QdrantInspectResponse } from '../../types'
import { formatCount } from '../../utils/format'

export default function QdrantStatsPanel({
  stats,
}: {
  stats: QdrantInspectResponse
}) {
  const collections = [
    { stat: stats.subjects, label: 'subjects (sparse)' },
    { stat: stats.objects, label: 'objects (sparse)' },
    { stat: stats.subjects_dense, label: 'subjects (dense)' },
    { stat: stats.objects_dense, label: 'objects (dense)' },
  ] as const

  return (
    <Panel title="Qdrant Collections">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {collections.map(({ stat, label }) => {
          return (
            <MetricTile
              key={label}
              label={<span className="truncate" title={label}>{label}</span>}
              value={stat.exists ? formatCount(stat.points_count) : '—'}
              sub={stat.exists ? 'pts' : undefined}
              valueClassName={stat.exists ? 'text-2xl' : 'text-sm'}
              className="bg-subtle"
            />
          )
        })}
      </div>
    </Panel>
  )
}
