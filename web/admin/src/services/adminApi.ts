import { api } from './api'
import type {
  BatchRunsResponse,
  EventsListResponse,
  HealthData,
  InjectEventRequest,
  MutationOkResponse,
  NamespaceConfig,
  NamespaceListResponse,
  NamespacesOverviewResponse,
  QdrantStatsResponse,
  RecommendDebugRequest,
  RecommendDebugResponse,
  SubjectProfileRequest,
  SubjectProfileResponse,
  TrendingAdminResponse,
  TriggerBatchResponse,
  UpsertNamespacePayload,
  UpsertNamespaceResponse,
} from '../types'

export interface EventsListParams {
  namespace: string
  limit: number
  offset: number
  subjectID?: string
}

export interface TrendingParams {
  namespace: string
  limit: number
  offset: number
  windowHours: number
}

export const adminApi = {
  getHealth: () => api.get<HealthData>('/api/admin/v1/health'),

  listNamespaces: () => api.get<NamespaceListResponse>('/api/admin/v1/namespaces'),

  getNamespace: (namespace: string) =>
    api.get<NamespaceConfig>(`/api/admin/v1/namespaces/${encodeURIComponent(namespace)}`),

  upsertNamespace: (namespace: string, payload: UpsertNamespacePayload) =>
    api.put<UpsertNamespaceResponse>(`/api/admin/v1/namespaces/${encodeURIComponent(namespace)}`, payload),

  getNamespacesOverview: () =>
    api.get<NamespacesOverviewResponse>('/api/admin/v1/namespaces/overview'),

  getQdrantStats: (namespace: string) =>
    api.get<QdrantStatsResponse>(`/api/admin/v1/namespaces/${encodeURIComponent(namespace)}/qdrant-stats`),

  listBatchRuns: (namespace?: string, limit = 20, offset = 0, status = '') => {
    const p = new URLSearchParams({ limit: String(limit), offset: String(offset) })
    if (namespace) p.set('namespace', namespace)
    if (status) p.set('status', status)
    return api.get<BatchRunsResponse>(`/api/admin/v1/batch-runs?${p}`)
  },

  triggerBatchRun: (namespace: string) =>
    api.post<TriggerBatchResponse>(`/api/admin/v1/namespaces/${encodeURIComponent(namespace)}/batch-runs/trigger`, {}),

  listEvents: ({ namespace, limit, offset, subjectID }: EventsListParams) => {
    const params = new URLSearchParams({
      limit: String(limit),
      offset: String(offset),
    })
    if (subjectID) params.set('subject_id', subjectID)

    return api.get<EventsListResponse>(`/api/admin/v1/namespaces/${encodeURIComponent(namespace)}/events?${params}`)
  },

  injectEvent: (namespace: string, event: InjectEventRequest) =>
    api.post<MutationOkResponse>(`/api/admin/v1/namespaces/${encodeURIComponent(namespace)}/events`, event),

  debugRecommendations: (request: RecommendDebugRequest) =>
    api.post<RecommendDebugResponse>('/api/admin/v1/recommend/debug', request),

  getSubjectProfile: ({ namespace, subject_id }: SubjectProfileRequest) =>
    api.get<SubjectProfileResponse>(`/api/admin/v1/subjects/${encodeURIComponent(namespace)}/${encodeURIComponent(subject_id)}/profile`),

  getTrending: ({ namespace, limit, offset, windowHours }: TrendingParams) => {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
    if (windowHours > 0) params.set('window_hours', String(windowHours))

    return api.get<TrendingAdminResponse>(`/api/admin/v1/trending/${encodeURIComponent(namespace)}?${params}`)
  },
}
