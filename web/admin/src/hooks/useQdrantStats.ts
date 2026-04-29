import { useQuery } from '@tanstack/react-query'
import { api } from '../services/api'

export interface QdrantCollectionStat {
  exists: boolean
  points_count: number
  indexed_vectors_count: number
}

export interface QdrantStatsResponse {
  namespace: string
  collections: Record<string, QdrantCollectionStat>
}

export function useQdrantStats(ns: string) {
  return useQuery<QdrantStatsResponse, Error>({
    queryKey: ['qdrant-stats', ns],
    queryFn: () => api.get(`/api/admin/v1/namespaces/${encodeURIComponent(ns)}/qdrant-stats`),
    enabled: !!ns && ns !== 'new',
    refetchInterval: 30_000,
  })
}
