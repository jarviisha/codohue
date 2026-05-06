import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'
import type { UpsertNamespacePayload } from '../types'

export function useNamespaceList() {
  return useQuery({
    queryKey: queryKeys.namespaces.list(),
    queryFn: adminApi.listNamespaces,
  })
}

export function useNamespace(ns: string) {
  return useQuery({
    queryKey: queryKeys.namespaces.detail(ns),
    queryFn: () => adminApi.getNamespace(ns),
    enabled: !!ns && ns !== 'new',
  })
}

export function useUpsertNamespace() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ ns, payload }: { ns: string; payload: UpsertNamespacePayload }) =>
      adminApi.upsertNamespace(ns, payload),
    onSuccess: (_result, { ns }) => {
      qc.invalidateQueries({ queryKey: queryKeys.namespaces.list() })
      qc.invalidateQueries({ queryKey: queryKeys.namespaces.overview() })
      qc.invalidateQueries({ queryKey: queryKeys.namespaces.detail(ns) })
    },
  })
}
