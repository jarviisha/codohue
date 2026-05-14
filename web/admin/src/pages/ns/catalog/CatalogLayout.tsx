import { Outlet, useNavigate, useParams } from 'react-router-dom'
import {
  Button,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  TabLink,
  Tabs,
  useRegisterCommand,
} from '@/components/ui'
import { useCatalogConfig } from '@/services/catalog'
import { paths } from '@/routes/path'
import type { CatalogContext } from './catalogContext'

// Shared layout for /ns/:name/catalog. Owns the namespace catalog query, draws
// the page header + section tabs, and renders the active child route through
// <Outlet> so the URL is the single source of truth for tab state.
export default function CatalogLayout() {
  const { name = '' } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const catalog = useCatalogConfig(name)

  useRegisterCommand(
    `ns.${name}.catalog.refresh`,
    `Refresh ${name} catalog`,
    () => void catalog.refetch(),
    name,
  )
  useRegisterCommand(
    `ns.${name}.catalog.status`,
    `Open ${name} catalog status`,
    () => navigate(paths.nsCatalog(name)),
    name,
  )
  useRegisterCommand(
    `ns.${name}.catalog.config`,
    `Open ${name} catalog config`,
    () => navigate(paths.nsCatalogConfig(name)),
    name,
  )
  useRegisterCommand(
    `ns.${name}.catalog.items`,
    `Open ${name} catalog items`,
    () => navigate(paths.nsCatalogItems(name)),
    name,
  )

  const refresh = () => void catalog.refetch()

  return (
    <PageShell>
      <PageHeader
        title="Catalog"
        meta={`namespace ${name}`}
        actions={
          <Button
            variant="ghost"
            size="sm"
            loading={catalog.isFetching}
            onClick={refresh}
          >
            Refresh
          </Button>
        }
      />

      <Tabs ariaLabel="Catalog sections">
        <TabLink to={paths.nsCatalog(name)} end>Status</TabLink>
        <TabLink to={paths.nsCatalogConfig(name)}>Config</TabLink>
        <TabLink to={paths.nsCatalogItems(name)}>Items</TabLink>
      </Tabs>

      {catalog.isLoading ? (
        <LoadingState rows={7} label="loading catalog" />
      ) : catalog.isError || !catalog.data ? (
        <Notice tone="fail" title="Failed to load catalog">
          {(catalog.error as Error)?.message ?? 'Unable to load catalog state.'}
        </Notice>
      ) : (
        <Outlet
          context={{
            data: catalog.data,
            refetch: refresh,
            isFetching: catalog.isFetching,
          } satisfies CatalogContext}
        />
      )}
    </PageShell>
  )
}
