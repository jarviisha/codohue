import { useMutation } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import type { SubjectProfileRequest } from '../types'

export function useSubjectProfile() {
  return useMutation({
    mutationFn: (request: SubjectProfileRequest) => adminApi.getSubjectProfile(request),
  })
}
