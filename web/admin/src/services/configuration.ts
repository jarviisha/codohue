import { useQuery } from '@tanstack/react-query'
import { apiFetch } from './http'

export const groupNames = ['recommendations', 'signals', 'trending', 'embeddings'] as const
export type GroupName = (typeof groupNames)[number]
export type Values = Record<string, unknown>
export type ConfigurationGroup = {
  revision: number
  values: Values
  locks: Record<string, string>
  guidance: string
}
export type DefaultObservation = {
  state: 'unknown' | 'mixed' | 'observed'
  process: string
  value?: number
  reports: { instance: string; reported_at: string; value: number }[]
}
export type Configuration = {
  defaults?: Record<string, DefaultObservation>
  namespace: string
  generation: number
  groups: Record<GroupName, ConfigurationGroup>
}
export const configurationPath = (ns: string) =>
  `/api/admin/v1/namespaces/${encodeURIComponent(ns)}/configuration`
export function useConfiguration(ns: string) {
  return useQuery({
    queryKey: ['configuration', ns],
    queryFn: () => apiFetch<Configuration>(configurationPath(ns)),
    refetchInterval: 30_000,
  })
}

export { toDraft, draftChanges, parseChanges } from './configurationDraft'
export type { Draft } from './configurationDraft'
