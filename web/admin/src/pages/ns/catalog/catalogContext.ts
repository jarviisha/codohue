import { useOutletContext } from 'react-router-dom'
import type { NamespaceCatalogResponse } from '@/services/catalog'

export interface CatalogContext {
  data: NamespaceCatalogResponse
  refetch: () => void
  isFetching: boolean
}

export function useCatalogContext(): CatalogContext {
  return useOutletContext<CatalogContext>()
}
