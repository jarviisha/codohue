import { useNavigate } from 'react-router-dom'
import { Banner, Button, Card, Stack } from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import {
  useClearDemoData,
  useSeedDemoData,
  type DemoDatasetResponse,
} from '@/services/dangerZone'
import PageHeader from '@/components/shell/PageHeader'
import NamespaceTag from '@/components/NamespaceTag'
import SecretValue from '@/components/SecretValue'

/**
 * DemoDataPage manages the bundled demo dataset — a single fixed namespace plus
 * sample events and catalog items. Both actions are idempotent and act on that
 * one namespace regardless of the current URL, so the page lives at the global
 * /demo-data route rather than under /ns/:ns. Kept out of the Danger zone since
 * seeding sample data is not destructive.
 */
export default function DemoDataPage() {
  const navigate = useNavigate()
  const seed = useSeedDemoData()
  const clear = useClearDemoData()

  return (
    <PageContainer size="md">
      <PageHeader>
        <Stack gap={1}>
          <h1 className="text-primary text-xl font-semibold">Demo data</h1>
          <p className="text-secondary text-sm">
            Seed or clear the bundled demo namespace — handy for kicking the tyres on a fresh
            install without wiring a real client.
          </p>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        <DemoCard
          title="Seed demo dataset"
          description="Creates the bundled demo namespace plus sample events and catalog items. Idempotent — re-runs reset the dataset back to its baseline state."
          action="Seed"
          loading={seed.isPending}
          error={seed.error}
          result={seed.data}
          onRun={() => seed.mutate()}
          onOpen={(ns) => navigate(`/ns/${encodeURIComponent(ns)}`)}
        />

        <DemoCard
          title="Clear demo dataset"
          description="Wipes the bundled demo namespace if present. Safe to run when the namespace doesn't exist."
          action="Clear"
          loading={clear.isPending}
          error={clear.error}
          result={clear.data}
          onRun={() => clear.mutate()}
        />
      </Stack>
    </PageContainer>
  )
}

function DemoCard({
  title,
  description,
  action,
  loading,
  error,
  result,
  onRun,
  onOpen,
}: {
  title: string
  description: string
  action: string
  loading: boolean
  error: Error | null
  result: DemoDatasetResponse | undefined
  onRun: () => void
  onOpen?: (ns: string) => void
}) {
  return (
    <Card>
        <Stack gap={6}>
          <span className="text-secondary text-xs uppercase tracking-wide">{title}</span>
          <p className="text-secondary text-sm">{description}</p>
          {error && <Banner status="error" title={`${title} failed`} description={error.message} />}
          {result && (
            <Banner
              status="success"
              title={`${title} complete`}
              description={describeDemoResult(result)}
              endContent={
                onOpen && (
                  <Button
                    size="sm"
                    variant="ghost"
                    label={`Open ${result.namespace}`}
                    onClick={() => onOpen(result.namespace)}
                  >
                    Open <NamespaceTag name={result.namespace} />
                  </Button>
                )
              }
            />
          )}
          {result?.api_key && (
            // Seeding mints the demo namespace's data-plane key, and the
            // backend returns the plaintext exactly once. Printing it here is
            // the only way an operator gets to keep it — otherwise recovering
            // it costs a key rotation.
            <Stack gap={1}>
              <span className="text-secondary text-xs uppercase tracking-wide">
                API key — shown once
              </span>
              <SecretValue value={result.api_key} label="demo namespace API key" />
            </Stack>
          )}
          <Stack align="center" gap={4} direction="horizontal" justify="end">
            <Button
              label={loading ? `${action.replace(/e$/, '')}ing…` : action}
              onClick={onRun}
              isDisabled={loading}
            />
          </Stack>
        </Stack>
    </Card>
  )
}

function describeDemoResult(r: DemoDatasetResponse): string {
  const bits: string[] = [`namespace ${r.namespace}`]
  if (r.events_created) bits.push(`${r.events_created.toLocaleString()} events created`)
  if (r.events_deleted) bits.push(`${r.events_deleted.toLocaleString()} events deleted`)
  if (r.catalog_items_created)
    bits.push(`${r.catalog_items_created.toLocaleString()} catalog items created`)
  if (r.api_key) bits.push('new api_key issued — copy it below')
  return bits.join(', ')
}
