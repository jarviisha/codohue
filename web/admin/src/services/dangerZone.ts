import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from './http'

// ---------------------------------------------------------------------------
// Wire types — mirror internal/admin/types.go.
// ---------------------------------------------------------------------------

export type DemoDatasetResponse = {
  namespace: string
  events_created?: number
  events_deleted?: number
  catalog_items_created?: number
  api_key?: string
}

export type NamespaceDeleteResponse = {
  namespace: string
  events_deleted: number
}

export type ResetAppResponse = {
  namespaces_deleted: number
  events_deleted: number
  namespaces: string[]
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export function useSeedDemoData() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      apiFetch<DemoDatasetResponse>('/api/admin/v1/demo-data', { method: 'POST' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['namespaces'] })
      qc.invalidateQueries({ queryKey: ['overview'] })
    },
  })
}

export function useClearDemoData() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      apiFetch<DemoDatasetResponse>('/api/admin/v1/demo-data', { method: 'DELETE' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['namespaces'] })
      qc.invalidateQueries({ queryKey: ['overview'] })
    },
  })
}

export function useDeleteNamespace() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (namespace: string) =>
      apiFetch<NamespaceDeleteResponse>(
        `/api/admin/v1/namespaces/${encodeURIComponent(namespace)}`,
        { method: 'DELETE' },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['namespaces'] })
      qc.invalidateQueries({ queryKey: ['overview'] })
    },
  })
}

export function useResetApp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      apiFetch<ResetAppResponse>('/api/admin/v1/reset', {
        method: 'POST',
        body: JSON.stringify({ confirm: 'RESET' }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['namespaces'] })
      qc.invalidateQueries({ queryKey: ['overview'] })
    },
  })
}
