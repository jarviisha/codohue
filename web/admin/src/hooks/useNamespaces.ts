import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../services/api'

export interface NamespaceConfig {
  namespace: string
  action_weights: Record<string, number>
  lambda: number
  gamma: number
  alpha: number
  max_results: number
  seen_items_days: number
  dense_strategy: string
  embedding_dim: number
  dense_distance: string
  trending_window: number
  trending_ttl: number
  lambda_trending: number
  has_api_key: boolean
  updated_at: string
}

export interface UpsertNamespacePayload {
  action_weights?: Record<string, number>
  lambda?: number
  gamma?: number
  alpha?: number
  max_results?: number
  seen_items_days?: number
  dense_strategy?: string
  embedding_dim?: number
  dense_distance?: string
  trending_window?: number
  trending_ttl?: number
  lambda_trending?: number
}

export interface UpsertNamespaceResponse {
  namespace: string
  updated_at: string
  api_key?: string
}

export function useNamespaceList() {
  return useQuery<{ namespaces: NamespaceConfig[] }>({
    queryKey: ['namespaces'],
    queryFn: () => api.get('/api/admin/v1/namespaces'),
  })
}

export function useNamespace(ns: string) {
  return useQuery<NamespaceConfig>({
    queryKey: ['namespaces', ns],
    queryFn: () => api.get(`/api/admin/v1/namespaces/${ns}`),
    enabled: !!ns && ns !== 'new',
  })
}

export function useUpsertNamespace() {
  const qc = useQueryClient()
  return useMutation<UpsertNamespaceResponse, Error, { ns: string; payload: UpsertNamespacePayload }>({
    mutationFn: ({ ns, payload }) => api.put(`/api/admin/v1/namespaces/${ns}`, payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['namespaces'] })
    },
  })
}
