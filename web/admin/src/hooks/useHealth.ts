import { useQuery } from '@tanstack/react-query'
import { api } from '../services/api'

export interface HealthData {
  postgres: string
  redis: string
  qdrant: string
  status: string
}

export function useHealth() {
  return useQuery<HealthData>({
    queryKey: ['health'],
    queryFn: () => api.get<HealthData>('/api/admin/v1/health'),
    refetchInterval: 10_000,
    retry: false,
  })
}
