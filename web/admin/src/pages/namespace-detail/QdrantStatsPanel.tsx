import { MetricTile, Panel } from '../../components/ui'
import type { QdrantCollectionStat } from '../../types'
import { formatCount } from '../../utils/format'

export default function QdrantStatsPanel({
  ns,
  stats,
}: {
  ns: string
  stats: Record<string, QdrantCollectionStat>
}) {
  const collections = [
    { key: `${ns}_subjects`, label: 'subjects (sparse)' },
    { key: `${ns}_objects`, label: 'objects (sparse)' },
    { key: `${ns}_subjects_dense`, label: 'subjects (dense)' },
    { key: `${ns}_objects_dense`, label: 'objects (dense)' },
  ]

  return (
    <Panel title="Qdrant Collections">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {collections.map(({ key, label }) => {
          const col = stats[key]
          return (
            <MetricTile
              key={key}
              label={<span className="truncate" title={key}>{label}</span>}
              value={col?.exists ? formatCount(col.points_count) : '—'}
              sub={col?.exists ? 'pts' : undefined}
              valueClassName={col?.exists ? 'text-2xl' : 'text-sm'}
              className="bg-subtle"
            />
          )
        })}
      </div>
    </Panel>
  )
}
