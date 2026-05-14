import { useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from './http'
import { namespaceKeys } from './namespaces'

// ────────────────────────────────────────────────────────────────────────────
// Wire types
// ────────────────────────────────────────────────────────────────────────────

export interface BatchRunCreateResponse {
  id: number
  namespace: string
}

// ────────────────────────────────────────────────────────────────────────────
// Query keys
// ────────────────────────────────────────────────────────────────────────────

export const batchRunKeys = {
  all: ['batchRuns'] as const,
  list: (namespace: string) => ['batchRuns', 'list', namespace] as const,
}

// ────────────────────────────────────────────────────────────────────────────
// Request functions
// ────────────────────────────────────────────────────────────────────────────

export function triggerBatchRun(namespace: string) {
  return http.post<BatchRunCreateResponse>(
    `/api/admin/v1/namespaces/${encodeURIComponent(namespace)}/batch-runs`,
    {},
  )
}

// ────────────────────────────────────────────────────────────────────────────
// Hooks
// ────────────────────────────────────────────────────────────────────────────

export function useTriggerBatchRun() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: triggerBatchRun,
    onSuccess: (_data, namespace) => {
      qc.invalidateQueries({ queryKey: namespaceKeys.overview() })
      qc.invalidateQueries({ queryKey: namespaceKeys.byName(namespace) })
      qc.invalidateQueries({ queryKey: batchRunKeys.list(namespace) })
    },
  })
}
