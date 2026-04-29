import { useMutation } from '@tanstack/react-query'
import { api } from '../services/api'

export interface SubjectProfileResponse {
  subject_id: string
  namespace: string
  interaction_count: number
  seen_items: string[]
  seen_items_days: number
  sparse_vector_nnz: number // -1 means not yet indexed in Qdrant
}

export function useSubjectProfile() {
  return useMutation<SubjectProfileResponse, Error, { namespace: string; subject_id: string }>({
    mutationFn: ({ namespace, subject_id }) =>
      api.get(`/api/admin/v1/subjects/${encodeURIComponent(namespace)}/${encodeURIComponent(subject_id)}/profile`),
  })
}
