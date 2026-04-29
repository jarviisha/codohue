import { useMutation } from '@tanstack/react-query'
import { api } from '../services/api'

export interface RecommendDebugRequest {
  namespace: string
  subject_id: string
  limit: number
  offset: number
}

export interface RecommendDebugItem {
  object_id: string
  score: number
  rank: number
}

export interface RecommendDebugResponse {
  subject_id: string
  namespace: string
  items: RecommendDebugItem[]
  source: string
  limit: number
  offset: number
  total: number
  generated_at: string
}

export function useRecommendDebug() {
  return useMutation<RecommendDebugResponse, Error, RecommendDebugRequest>({
    mutationFn: (req) => api.post('/api/admin/v1/recommend/debug', req),
  })
}
