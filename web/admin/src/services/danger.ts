import { useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from './http'
import { namespaceKeys } from './namespaces'

// resetApp wipes every namespace + its data across postgres, redis, and
// qdrant. The backend rejects any payload other than {"confirm":"RESET"}, so
// the UI must echo the literal sentinel — see ConfirmDialog requireTyped.

export interface ResetAppResponse {
  namespaces_deleted: number
  events_deleted: number
  namespaces: string[]
}

export function resetApp() {
  return http.post<ResetAppResponse>('/api/admin/v1/reset', { confirm: 'RESET' })
}

export function useResetApp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: resetApp,
    onSuccess: () => {
      // Every cached namespace query is now empty.
      qc.invalidateQueries({ queryKey: namespaceKeys.all })
    },
  })
}
