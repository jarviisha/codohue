// Centralized URL builders. Use these instead of hand-stringing paths so renames
// stay grep-able and consistent with BUILD_PLAN.md §3.
export const paths = {
  login: '/login',
  health: '/',
  namespaces: '/namespaces',
  namespaceCreate: '/namespaces/new',
  ns: (name: string) => `/ns/${name}`,
  nsConfig: (name: string) => `/ns/${name}/config`,
  nsCatalog: (name: string) => `/ns/${name}/catalog`,
  nsCatalogItems: (name: string) => `/ns/${name}/catalog/items`,
  nsCatalogItem: (name: string, id: string) => `/ns/${name}/catalog/items/${id}`,
  nsEvents: (name: string) => `/ns/${name}/events`,
  nsTrending: (name: string) => `/ns/${name}/trending`,
  nsBatchRuns: (name: string) => `/ns/${name}/batch-runs`,
  nsDebug: (name: string) => `/ns/${name}/debug`,
  nsDemoData: (name: string) => `/ns/${name}/demo-data`,
} as const
