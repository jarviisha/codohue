import { useMutation } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import type { RecommendDebugRequest } from '../types'

export function useRecommendDebug() {
  return useMutation({
    mutationFn: (req: RecommendDebugRequest) => adminApi.debugRecommendations(req),
  })
}
