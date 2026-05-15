import { useQuery } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'
import type { CatalogItemState } from '../types'

export interface UseCatalogItemsOptions {
  namespace: string
  state: CatalogItemState | 'all'
  limit: number
  offset: number
  objectID?: string
}

export function useCatalogItems({ namespace, state, limit, offset, objectID = '' }: UseCatalogItemsOptions) {
  return useQuery({
    queryKey: queryKeys.catalog.items(namespace, state, limit, offset, objectID),
    queryFn: () =>
      adminApi.listCatalogItems({
        namespace,
        state,
        limit,
        offset,
        objectID: objectID || undefined,
      }),
    enabled: namespace !== '',
    staleTime: 5_000,
  })
}

export function useCatalogItemDetail(namespace: string, id: number | null) {
  return useQuery({
    queryKey: queryKeys.catalog.item(namespace, id ?? 0),
    queryFn: () => adminApi.getCatalogItem(namespace, id as number),
    enabled: namespace !== '' && id !== null && id > 0,
    staleTime: 5_000,
  })
}
