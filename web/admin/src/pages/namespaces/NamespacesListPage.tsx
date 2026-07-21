import { Link, useSearchParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Button,
  Container,
  EmptyState,
  Inline,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from '@jarviisha/davinci-react-ui'
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
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Inline align="center" justify="between" className="w-full">
          <Stack gap="050">
            <h1 className="text-foreground text-xl font-semibold">Namespaces</h1>
            <p className="text-foreground-subtle text-sm">
              {q.data?.total ?? 0} configured. Click a row to open its overview.
            </p>
          </Stack>
          <Button onClick={() => setCreateOpen(true)}>New namespace</Button>
        </Inline>
      </PageHeader>

      <CreateNamespaceDialog open={createOpen} onOpenChange={setCreateOpen} />

      <Stack>
        {q.isLoading && <Skeleton className="h-48 w-full" />}

        {q.isError && (
          <Alert variant="danger" title="Failed to load namespaces" description={q.error?.message ?? ''} />
        )}

        {q.isSuccess && q.data.items.length === 0 && (
          <EmptyState
            title="No namespaces yet"
            description="Create your first namespace to start ingesting events and recommendations."
          />
        )}

        {q.isSuccess && q.data.items.length > 0 && (
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Namespace</TableHead>
                  <TableHead>Dense source</TableHead>
                  <TableHead align="right">Embedding dim</TableHead>
                  <TableHead>Catalog</TableHead>
                  <TableHead>Updated</TableHead>
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
                      <Badge variant="neutral">{ns.dense_source || '—'}</Badge>
                    </TableCell>
                    <TableCell align="right" className="tabular-nums">
                      {ns.embedding_dim}
                    </TableCell>
                    <TableCell>
                      {ns.dense_source === 'catalog' ? (
                        <Inline align="center">
                          <Badge variant="success">on</Badge>
                          {ns.catalog_strategy_id && (
                            <span className="text-foreground-subtle text-xs">
                              {ns.catalog_strategy_id}@{ns.catalog_strategy_version}
                            </span>
                          )}
                        </Inline>
                      ) : (
                        <Badge variant="neutral">off</Badge>
                      )}
                    </TableCell>
                    <TableCell className="text-foreground-subtle text-sm">
                      {new Date(ns.updated_at).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Stack>
    </Container>
  )
}
