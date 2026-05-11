import { useMutation, useQueryClient } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'

// useRedriveCatalogItem re-drives a single failed / dead-letter catalog item.
// Returns 202 + the row state on success; on success, the items list is
// invalidated so the UI shows the row back in 'pending'.
export function useRedriveCatalogItem(namespace: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => adminApi.redriveCatalogItem(namespace, id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['catalog-items', namespace] })
      queryClient.invalidateQueries({ queryKey: ['catalog-item', namespace] })
    },
  })
}

// useBulkRedriveDeadletter resets every dead_letter row in the namespace to
// 'pending' and re-publishes them to the embed stream. SC-008.
export function useBulkRedriveDeadletter(namespace: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => adminApi.bulkRedriveDeadletter(namespace),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['catalog-items', namespace] })
    },
  })
}

// useDeleteCatalogItem removes a catalog row from Postgres and best-effort
// removes the matching dense vector from Qdrant. Idempotent.
export function useDeleteCatalogItem(namespace: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => adminApi.deleteCatalogItem(namespace, id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['catalog-items', namespace] })
    },
  })
}
