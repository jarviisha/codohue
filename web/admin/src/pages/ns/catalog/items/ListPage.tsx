import { Outlet } from 'react-router-dom'
import PageStub from '../../../_stub'

// Renders the items list page + an Outlet so /ns/:name/catalog/items/:id can
// open as a modal route layered over the list.
export default function CatalogItemsListPage() {
  return (
    <>
      <PageStub title="Catalog items" />
      <Outlet />
    </>
  )
}
