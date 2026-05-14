import { Link, useNavigate } from 'react-router-dom'
import {
  Button,
  EmptyState,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  StatusToken,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
  useRegisterCommand,
} from '../../components/ui'
import {
  lastRunToken,
  namespaceStatusToken,
  useNamespacesOverview,
} from '../../services/namespaces'
import { paths } from '../../routes/path'
import { formatDurationMs, formatNumber, formatRelative } from '../../utils/format'

export default function NamespacesListPage() {
  const navigate = useNavigate()
  const { data, isLoading, isError, error, refetch, isFetching } =
    useNamespacesOverview()

  useRegisterCommand(
    'namespaces.create',
    'Create namespace',
    () => navigate(paths.namespaceCreate),
    'global',
  )
  useRegisterCommand(
    'namespaces.refresh',
    'Refresh namespaces list',
    () => {
      void refetch()
    },
    'global',
  )

  const items = data?.items ?? []

  return (
    <PageShell>
      <PageHeader
        title="[namespaces]"
        meta={data ? `${data.total} total` : null}
        actions={
          <>
            <Button
              variant="ghost"
              size='sm'
              loading={isFetching && !isLoading}
              onClick={() => void refetch()}
            >
              Refresh
            </Button>
            <Button
              size='sm'
              variant="primary"
              onClick={() => navigate(paths.namespaceCreate)}
            >
              + Create namespace
            </Button>
          </>
        }
      />

      {isError ? (
        <Notice tone="fail" title="Failed to load namespaces">
          {(error as Error)?.message ?? 'Unable to reach the admin API.'}
        </Notice>
      ) : null}

      {isLoading ? (
        <LoadingState rows={4} label="loading namespaces" />
      ) : items.length === 0 && !isError ? (
        <EmptyState
          title="No namespaces yet"
          description="Create the first namespace to start ingesting events and serving recommendations."
          action={
            <Button
              variant="primary"
              onClick={() => navigate(paths.namespaceCreate)}
            >
              Create your first namespace
            </Button>
          }
        />
      ) : (
        <Table>
            <Thead>
              <Tr>
                <Th>status</Th>
                <Th>namespace</Th>
                <Th align="right">events 24h</Th>
                <Th>last run</Th>
                <Th>updated</Th>
              </Tr>
            </Thead>
            <Tbody>
              {items.map((h) => (
                <Tr key={h.config.namespace}>
                  <Td>
                    <StatusToken
                      state={namespaceStatusToken(h.status)}
                      title={h.status}
                    />
                  </Td>
                  <Td mono>
                    <Link
                      to={paths.ns(h.config.namespace)}
                      className="hover:text-accent"
                    >
                      {h.config.namespace}
                    </Link>
                  </Td>
                  <Td mono align="right">
                    {formatNumber(h.active_events_24h)}
                  </Td>
                  <Td>
                    {h.last_run ? (
                      <span className="inline-flex items-center gap-2">
                        <StatusToken
                          state={lastRunToken(h.last_run)}
                          title={
                            h.last_run.success
                              ? 'success'
                              : h.last_run.error_message ?? 'failed'
                          }
                        />
                        <span className="font-mono text-xs text-muted">
                          {formatRelative(h.last_run.started_at)}
                          {h.last_run.duration_ms !== null
                            ? ` · ${formatDurationMs(h.last_run.duration_ms)}`
                            : ''}
                        </span>
                      </span>
                    ) : (
                      <StatusToken state="idle" title="no batch run yet" />
                    )}
                  </Td>
                  <Td>
                    <span className="font-mono text-xs text-muted">
                      {formatRelative(h.config.updated_at)}
                    </span>
                  </Td>
                </Tr>
              ))}
            </Tbody>
        </Table>
      )}
    </PageShell>
  )
}
