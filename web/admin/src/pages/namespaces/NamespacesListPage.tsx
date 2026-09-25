import { Link, useSearchParams } from 'react-router-dom'
import {
  Banner,
  Button,
  EmptyState,
  Skeleton,
  Stack,
  StatusDot,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  Token,
} from '@astryxdesign/core'
import { useNamespaces } from '@/services/namespaces'
import PageHeader from '@/components/shell/PageHeader'
import CreateNamespaceDialog from '@/pages/namespaces/CreateNamespaceDialog'
import NamespaceTag from '@/components/NamespaceTag'

export default function NamespacesListPage() {
  const q = useNamespaces()
  const [searchParams, setSearchParams] = useSearchParams()

  // The URL is the single source of truth for the dialog: the command palette
  // deep-links via `/namespaces?new=1`, and opening/closing just sets or clears
  // the param. No mirrored state, so no effect-based sync.
  const createOpen = searchParams.get('new') === '1'
  const setCreateOpen = (open: boolean) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (open) next.set('new', '1')
        else next.delete('new')
        return next
      },
      { replace: true },
    )
  }

  return (
    <>
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full">
          <Stack gap={1}>
            <h1 className="text-primary text-xl font-semibold">Namespaces</h1>
            <p className="text-secondary text-sm">
              {q.data?.total ?? 0} configured. Click a row to open its overview.
            </p>
          </Stack>
          <Button onClick={() => setCreateOpen(true)} label="New namespace" />
        </Stack>
      </PageHeader>

      <CreateNamespaceDialog open={createOpen} onOpenChange={setCreateOpen} />

      <Stack gap={6}>
        {q.isLoading && <Skeleton height={192} />}

        {q.isError && (
          <Banner status="error" title="Failed to load namespaces" description={q.error?.message ?? ''} />
        )}

        {q.isSuccess && q.data.items.length === 0 && (
          <EmptyState
            title="No namespaces yet"
            description="Create your first namespace to start ingesting events and recommendations."
          />
        )}

        {q.isSuccess && q.data.items.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHeaderCell>Namespace</TableHeaderCell>
                  <TableHeaderCell>Dense source</TableHeaderCell>
                  <TableHeaderCell className="text-right" >Embedding dim</TableHeaderCell>
                  <TableHeaderCell>Catalog</TableHeaderCell>
                  <TableHeaderCell>Updated</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {q.data.items.map((ns) => (
                  <TableRow key={ns.namespace}>
                    <TableCell>
                      <Link
                        to={`/ns/${encodeURIComponent(ns.namespace)}`}
                        className="font-medium"
                      >
                        <NamespaceTag name={ns.namespace} />
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Token color="gray" label={ns.dense_source || '—'} />
                    </TableCell>
                    <TableCell  className="text-right tabular-nums">
                      {ns.embedding_dim}
                    </TableCell>
                    <TableCell>
                      {ns.dense_source === 'catalog' ? (
                        <Stack gap={4} direction="horizontal" align="center">
                          <StatusDot variant="success" label="catalog on" tooltip="catalog on" />
                          {ns.catalog_strategy_id && (
                            <span className="text-secondary text-xs">
                              {ns.catalog_strategy_id}@{ns.catalog_strategy_version}
                            </span>
                          )}
                        </Stack>
                      ) : (
                        <span className="text-secondary text-sm">off</span>
                      )}
                    </TableCell>
                    <TableCell className="text-secondary text-sm">
                      {new Date(ns.updated_at).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
        )}
      </Stack>
    </>
  )
}
