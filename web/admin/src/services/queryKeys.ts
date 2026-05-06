export const queryKeys = {
  health: () => ['health'] as const,

  namespaces: {
    list: () => ['namespaces'] as const,
    detail: (namespace: string) => ['namespaces', namespace] as const,
    overview: () => ['namespaces-overview'] as const,
    qdrantStats: (namespace: string) => ['qdrant-stats', namespace] as const,
  },

  batchRuns: {
    list: (namespace?: string, offset = 0, status = '') => ['batch-runs', namespace ?? '', offset, status] as const,
  },

  events: {
    list: (namespace: string, limit: number, offset: number, subjectID: string) =>
      ['events', namespace, limit, offset, subjectID] as const,
    namespace: (namespace: string) => ['events', namespace] as const,
  },

  trending: {
    list: (namespace: string, limit: number, offset: number, windowHours: number) =>
      ['trending', namespace, limit, offset, windowHours] as const,
  },
}
