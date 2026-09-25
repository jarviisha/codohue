import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  Badge,
  Banner,
  Button,
  Card,
  Dialog,
  DialogHeader,
  Layout,
  LayoutContent,
  LayoutFooter,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  TextInput,
} from '@astryxdesign/core'
import { useNamespaceDashboard } from '@/services/namespaces'
import { useDeleteNamespace } from '@/services/dangerZone'
import PageHeader from '@/components/shell/PageHeader'
import PhaseStrip from '@/components/monitoring/PhaseStrip'
import MetaLine from '@/components/MetaLine'

export default function NamespaceOverviewPage() {
  const { ns } = useParams<{ ns: string }>()
  const navigate = useNavigate()
  const q = useNamespaceDashboard(ns ?? null)
  const [deleteOpen, setDeleteOpen] = useState(false)

  if (!ns) return null

  if (q.isLoading) {
    return <Skeleton height={192} />
  }

  if (q.isError) {
    return (
      <Banner
        status="error"
        title="Could not load namespace"
        description={q.error?.message ?? 'unknown error'}
      />
    )
  }

  const data = q.data
  if (!data) {
    return (
      <Banner
        status="warning"
        title="Empty namespace response"
        description="Backend returned no data for this namespace — verify the admin binary is on the latest commit."
      />
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
    <>
      <PageHeader>
        <Stack gap={4} direction="horizontal" align="center" justify="between" className="w-full" wrap="wrap">
          <Stack gap={1}>
            <Stack gap={4} direction="horizontal" align="center">
              <h1 className="text-primary text-xl font-semibold">Overview</h1>
              {config?.dense_source === 'catalog' && <Badge variant="success" label="catalog" />}
            </Stack>
            {config && (
              <MetaLine
                items={[
                  `dense_source=${config.dense_source}`,
                  `embedding_dim=${config.embedding_dim}`,
                  `alpha=${config.alpha}`,
                  `λ=${config.lambda}`,
                ]}
              />
            )}
          </Stack>
          <Stack gap={4} direction="horizontal" align="center">
            <Button
              size="sm"
              variant="secondary"
              
              onClick={() => navigate(`/ns/${encodeURIComponent(data.namespace)}/events`)} label="Events →" />
            <Button
              size="sm"
              variant="secondary"
              
              onClick={() => navigate(`/ns/${encodeURIComponent(data.namespace)}/subjects`)} label="Inspect subject →" />
            <Button
              size="sm"
              variant="secondary"
              
              onClick={() => navigate(`/ns/${encodeURIComponent(data.namespace)}/config`)} label="Configure →" />
          </Stack>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        <Stack gap={4} direction="horizontal" align="start" wrap="wrap">
          <Tile label="Events (24h)" value={events24h.toLocaleString()} />
          <Tile label="Events / min" value={eventsPerMin.toFixed(1)} />
          <Tile label="Sparse subjects" value={subjectsCount.toLocaleString()} />
          <Tile label="Sparse objects" value={objectsCount.toLocaleString()} />
          {config?.dense_source === 'catalog' && (
            <Tile
              label="Catalog backlog"
              value={catalog.pending.toLocaleString()}
              hint={`${catalog.dead_letter} DL`}
            />
          )}
        </Stack>

        <Stack gap={6}>
          <Stack gap={6}>
            <h2 className="text-primary text-sm font-semibold">Last batch runs</h2>
            <p className="text-secondary text-xs">
              Twelve most recent runs across CF and re-embed kinds.
            </p>
          </Stack>
          {lastRuns.length === 0 ? (
            <p className="text-secondary text-sm">No runs yet.</p>
          ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHeaderCell>Run</TableHeaderCell>
                    <TableHeaderCell>Kind</TableHeaderCell>
                    <TableHeaderCell>Started</TableHeaderCell>
                    <TableHeaderCell>Phases</TableHeaderCell>
                    <TableHeaderCell className="text-right" >Duration</TableHeaderCell>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {lastRuns.map((r) => (
                    <TableRow key={r.id}>
                      <TableCell>
                        <Link to={`/ns/${encodeURIComponent(ns)}/batch-runs/${r.id}`} className="text-primary font-medium">
                          #{r.id}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <Badge variant="neutral" label={r.kind} />
                      </TableCell>
                      <TableCell className="text-secondary text-sm">
                        {new Date(r.started_at).toLocaleString()}
                      </TableCell>
                      <TableCell>
                        <PhaseStrip phaseStatus={r.phase_status} />
                      </TableCell>
                      <TableCell  className="text-right tabular-nums">
                        {r.duration_ms != null ? `${(r.duration_ms / 1000).toFixed(1)}s` : '—'}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
          )}
        </Stack>

        <Card>
            <Stack gap={4} direction="horizontal" align="center" justify="between" wrap="wrap">
              <Stack gap={6}>
                <span className="text-secondary text-xs uppercase tracking-wide">
                  Danger zone
                </span>
                <p className="text-secondary text-sm">
                  Wipe this namespace and every trace of its data across Postgres, Redis, and
                  Qdrant. Cannot be undone.
                </p>
              </Stack>
              <Button  variant="destructive" onClick={() => setDeleteOpen(true)} label="Delete namespace…" />
            </Stack>
        </Card>
      </Stack>

      <DeleteNamespaceDialog
        namespace={data.namespace}
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onSuccess={() => {
          setDeleteOpen(false)
          navigate('/namespaces')
        }}
      />
    </>
  )
}

function DeleteNamespaceDialog({
  namespace,
  open,
  onOpenChange,
  onSuccess,
}: {
  namespace: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}) {
  return (
    <Dialog isOpen={open} onOpenChange={onOpenChange} width={560} purpose="required">
      {open && (
        <DeleteNamespaceForm
          namespace={namespace}
          onClose={() => onOpenChange(false)}
          onSuccess={onSuccess}
        />
      )}
    </Dialog>
  )
}

function DeleteNamespaceForm({
  namespace,
  onClose,
  onSuccess,
}: {
  namespace: string
  onClose: () => void
  onSuccess: () => void
}) {
  const del = useDeleteNamespace()
  const [confirm, setConfirm] = useState('')

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    del.mutate(namespace, {
      onSuccess: () => onSuccess(),
    })
  }

  return (
    <form onSubmit={onSubmit} className="contents">
      <Layout
        header={
          <DialogHeader
            title={`Delete namespace ${namespace}`}
            subtitle="Drops every event, vector, catalog item, and trending entry for this namespace. Cannot be undone — type the namespace name to confirm."
            onOpenChange={onClose}
          />
        }
        content={
          <LayoutContent>
            <Stack gap={6}>
              {del.error && (
                <Banner status="error" title="Delete failed" description={del.error.message} />
              )}
              <TextInput
                label={`Type "${namespace}" to confirm`}
                value={confirm}
                onChange={setConfirm}
                placeholder={namespace}
              />
            </Stack>
          </LayoutContent>
        }
        footer={
          <LayoutFooter>
            <Stack direction="horizontal" gap={2} align="center" hAlign="end">
              <Button type="button" variant="ghost" onClick={onClose} label="Cancel" />
              <Button
                variant="destructive"
                type="submit"
                isDisabled={confirm !== namespace || del.isPending}
                label={del.isPending ? 'Deleting…' : 'Delete namespace'}
              />
            </Stack>
          </LayoutFooter>
        }
      />
    </form>
  )
}

function Tile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Card className="flex-1 min-w-36">
        <Stack gap={6}>
          <span className="text-secondary text-xs uppercase tracking-wide">{label}</span>
          <Stack gap={4} direction="horizontal" align="center">
            <span className="text-primary text-xl font-semibold tabular-nums">{value}</span>
            {hint && <span className="text-secondary text-xs">{hint}</span>}
          </Stack>
        </Stack>
    </Card>
  )
}
