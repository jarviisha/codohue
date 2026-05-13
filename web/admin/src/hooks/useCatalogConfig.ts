import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'
import type { NamespaceCatalogUpdateRequest } from '../types'

// useCatalogConfig polls the per-namespace catalog config snapshot. Backlog
// counts and stream_len are refreshed every 10s so the operator sees the
// embed pipeline drain without manually reloading.
export function useCatalogConfig(namespace: string) {
  return useQuery({
    queryKey: queryKeys.catalog.config(namespace),
    queryFn: () => adminApi.getCatalogConfig(namespace),
    enabled: namespace !== '',
    staleTime: 5_000,
    refetchInterval: 10_000,
  })
}

// useUpdateCatalogConfig submits the form to PUT /catalog. On success the
// cached config is invalidated so the form re-reads the canonical state.
// Errors propagate to the caller; the form pulls the dim-mismatch payload
// out of ApiError.body.
export function useUpdateCatalogConfig(namespace: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: NamespaceCatalogUpdateRequest) =>
      adminApi.updateCatalogConfig(namespace, req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.catalog.config(namespace) })
      qc.invalidateQueries({ queryKey: queryKeys.namespaces.detail(namespace) })
    },
  })
}
