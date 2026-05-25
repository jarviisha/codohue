import { Link } from 'react-router-dom'
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

export default function NamespacesListPage() {
  const q = useNamespaces()

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Inline gap="200" align="center" justify="between" className="w-full">
          <Stack gap="025">
            <h1 className="text-foreground text-xl font-semibold">Namespaces</h1>
            <p className="text-foreground-subtle text-sm">
              {q.data?.total ?? 0} configured · click a row to open its overview.
            </p>
          </Stack>
          <Link to="/namespaces/new">
            <Button>New namespace</Button>
          </Link>
        </Inline>
      </PageHeader>

      <Stack gap="300">
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
                  <TableHead>Dense strategy</TableHead>
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
                        className="text-foreground font-medium"
                      >
                        {ns.namespace}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Badge variant="neutral">{ns.dense_strategy || '—'}</Badge>
                    </TableCell>
                    <TableCell align="right" className="tabular-nums">
                      {ns.embedding_dim}
                    </TableCell>
                    <TableCell>
                      {ns.catalog_enabled ? (
                        <Inline gap="050" align="center">
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
