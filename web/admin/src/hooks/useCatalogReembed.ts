import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import type { BatchRunLog, BatchRunsResponse } from '../types'

// useTriggerCatalogReEmbed kicks off a namespace-wide re-embed via
// POST /api/admin/v1/namespaces/{ns}/catalog/re-embed.
// On success, both batch_runs (so the operator can watch progress) and the
// catalog items list are invalidated so the UI reflects the new state.
export function useTriggerCatalogReEmbed(namespace: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => adminApi.triggerCatalogReEmbed(namespace),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['batch-runs'] })
      queryClient.invalidateQueries({ queryKey: ['catalog-items', namespace] })
      queryClient.invalidateQueries({ queryKey: ['catalog-last-reembed', namespace] })
    },
  })
}

// useLastReembedRun fetches the most recent admin_reembed batch_run_logs row
// for a namespace. Used to render in-progress / last-run state next to the
// re-embed button. Polls every 3s while the latest run is still open so the
// UI updates when cmd/embedder's watcher closes the row.
export function useLastReembedRun(namespace: string) {
  return useQuery<BatchRunLog | null>({
    queryKey: ['catalog-last-reembed', namespace],
    queryFn: async () => {
      const resp: BatchRunsResponse = await adminApi.listBatchRuns(namespace, 20, 0, '')
      const reembed = resp.items.find(r => r.trigger_source === 'admin_reembed')
      return reembed ?? null
    },
    enabled: namespace !== '',
    staleTime: 1_000,
    refetchInterval: query => {
      const data = query.state.data
      if (!data) return false
      return data.completed_at == null ? 3_000 : false
    },
  })
}
