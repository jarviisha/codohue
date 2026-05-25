import { Link, useParams } from 'react-router-dom'
import {
  Alert,
  Badge,
  Card,
  CardContent,
  Container,
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
import { useNamespaceDashboard } from '@/services/namespaces'
import PageHeader from '@/components/shell/PageHeader'
import PhaseStrip from '@/components/monitoring/PhaseStrip'

export default function NamespaceOverviewPage() {
  const { ns } = useParams<{ ns: string }>()
  const q = useNamespaceDashboard(ns ?? null)

  if (q.isLoading) {
    return (
      <Container size="full" className="py-6 px-6">
        <Skeleton className="h-48 w-full" />
      </Container>
    )
  }

  if (q.isError) {
    return (
      <Container size="full" className="py-6 px-6">
        <Alert
          variant="danger"
          title="Could not load namespace"
          description={q.error?.message ?? 'unknown error'}
        />
      </Container>
    )
  }

  const data = q.data
  if (!data) {
    return (
      <Container size="full" className="py-6 px-6">
        <Alert
          variant="warning"
          title="Empty namespace response"
          description="Backend returned no data for this namespace — verify the admin binary is on the latest commit."
        />
      </Container>
    )
  }

  // Defensive defaults — older backends or partial responses can omit any of
  // these nested fields; rendering "0" or "—" is preferable to a crash.
  const events24h = data.events_24h ?? 0
  const eventsPerMin = data.events_per_min_now ?? 0
  const subjectsCount = data.qdrant?.subjects?.points_count ?? 0
  const objectsCount = data.qdrant?.objects?.points_count ?? 0
  const catalog = data.catalog ?? { pending: 0, in_flight: 0, embedded: 0, failed: 0, dead_letter: 0, stream_len: 0 }
  const lastRuns = data.last_runs ?? []
  const config = data.config

  return (
    <Container size="full" className="py-6 px-6">
      <PageHeader>
        <Stack gap="025">
          <Inline gap="100" align="center">
            <h1 className="text-foreground text-xl font-semibold">{data.namespace}</h1>
            {config?.catalog_enabled && <Badge variant="success">catalog</Badge>}
          </Inline>
          {config && (
            <p className="text-foreground-subtle text-sm">
              dense_strategy={config.dense_strategy} · embedding_dim={config.embedding_dim} ·
              alpha={config.alpha} · λ={config.lambda}
            </p>
          )}
        </Stack>
      </PageHeader>

      <Stack gap="300">
        <Inline gap="200" align="start" wrap>
          <Tile label="Events (24h)" value={events24h.toLocaleString()} />
          <Tile label="Events / min" value={eventsPerMin.toFixed(1)} />
          <Tile label="Sparse subjects" value={subjectsCount.toLocaleString()} />
          <Tile label="Sparse objects" value={objectsCount.toLocaleString()} />
          {config?.catalog_enabled && (
            <Tile
              label="Catalog backlog"
              value={catalog.pending.toLocaleString()}
              hint={`${catalog.dead_letter} DL`}
            />
          )}
        </Inline>

        <Stack gap="100">
          <Stack gap="025">
            <h2 className="text-foreground text-sm font-semibold">Last batch runs</h2>
            <p className="text-foreground-subtle text-xs">
              Twelve most recent runs across CF and re-embed kinds.
            </p>
          </Stack>
          {lastRuns.length === 0 ? (
            <p className="text-foreground-subtle text-sm">No runs yet.</p>
          ) : (
            <TableContainer>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Run</TableHead>
                    <TableHead>Kind</TableHead>
                    <TableHead>Started</TableHead>
                    <TableHead>Phases</TableHead>
                    <TableHead align="right">Duration</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {lastRuns.map((r) => (
                    <TableRow key={r.id}>
                      <TableCell>
                        <Link to={`/batch-runs/${r.id}`} className="text-foreground font-medium">
                          #{r.id}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <Badge variant="neutral">{r.kind}</Badge>
                      </TableCell>
                      <TableCell className="text-foreground-subtle text-sm">
                        {new Date(r.started_at).toLocaleString()}
                      </TableCell>
                      <TableCell>
                        <PhaseStrip phaseStatus={r.phase_status} />
                      </TableCell>
                      <TableCell align="right" className="tabular-nums">
                        {r.duration_ms != null ? `${(r.duration_ms / 1000).toFixed(1)}s` : '—'}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Stack>
      </Stack>
    </Container>
  )
}

function Tile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Card className="flex-1 min-w-35">
      <CardContent>
        <Stack gap="025">
          <span className="text-foreground-subtle text-xs uppercase tracking-wide">{label}</span>
          <Inline gap="050" align="center">
            <span className="text-foreground text-xl font-semibold tabular-nums">{value}</span>
            {hint && <span className="text-foreground-subtle text-xs">{hint}</span>}
          </Inline>
        </Stack>
      </CardContent>
    </Card>
  )
}
