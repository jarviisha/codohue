/**
 * Centralised TanStack Query keys. Keep the literal tuples here so refactors
 * stay grep-able and invalidation across services lines up (e.g. mutating a
 * namespace invalidates the overview + dashboard keys).
 */
export const queryKeys = {
  session: ['session'] as const,
  health: ['health'] as const,
  overview: ['overview'] as const,
  namespaces: ['namespaces'] as const,
  namespaceDashboard: (ns: string) => ['namespaces', ns, 'dashboard'] as const,
  batchRuns: (filter?: Record<string, unknown>) =>
    ['batch-runs', filter ?? {}] as const,
  batchRunDetail: (id: number | string) => ['batch-runs', id] as const,
  batchRunStats: (window: string, bucket: string) =>
    ['batch-runs', 'stats', { window, bucket }] as const,
}
