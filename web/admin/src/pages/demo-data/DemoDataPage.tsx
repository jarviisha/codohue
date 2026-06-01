import { useNavigate } from 'react-router-dom'
import {
  Alert,
  Button,
  Card,
  CardContent,
  Container,
  Inline,
  Stack,
} from '@jarviisha/davinci-react-ui'
import {
  useClearDemoData,
  useSeedDemoData,
  type DemoDatasetResponse,
} from '@/services/dangerZone'
import PageHeader from '@/components/shell/PageHeader'

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
    <Container size="md" className="py-6">
      <PageHeader>
        <Stack gap="025">
          <h1 className="text-foreground text-xl font-semibold">Demo data</h1>
          <p className="text-foreground-subtle text-sm">
            Seed or clear the bundled demo namespace — handy for kicking the tyres on a fresh
            install without wiring a real client.
          </p>
        </Stack>
      </PageHeader>

      <Stack gap="300">
        <DemoCard
          title="Seed demo dataset"
          description="Creates the bundled demo namespace plus sample events and catalog items. Idempotent — re-runs reset the dataset back to its baseline state."
          action="Seed"
          tone="primary"
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
          tone="danger"
          loading={clear.isPending}
          error={clear.error}
          result={clear.data}
          onRun={() => clear.mutate()}
        />
      </Stack>
    </Container>
  )
}

function DemoCard({
  title,
  description,
  action,
  tone,
  loading,
  error,
  result,
  onRun,
  onOpen,
}: {
  title: string
  description: string
  action: string
  tone: 'primary' | 'danger'
  loading: boolean
  error: Error | null
  result: DemoDatasetResponse | undefined
  onRun: () => void
  onOpen?: (ns: string) => void
}) {
  return (
    <Card>
      <CardContent>
        <Stack gap="100">
          <span className="text-foreground-subtle text-xs uppercase tracking-wide">{title}</span>
          <p className="text-foreground-subtle text-sm">{description}</p>
          {error && <Alert variant="danger" title={`${title} failed`} description={error.message} />}
          {result && (
            <Alert
              variant="success"
              title={`${title} complete`}
              description={describeDemoResult(result)}
              actions={
                onOpen && (
                  <Button size="sm" variant="ghost" onClick={() => onOpen(result.namespace)}>
                    Open {result.namespace}
                  </Button>
                )
              }
            />
          )}
          <Inline gap="100" justify="end">
            <Button tone={tone === 'danger' ? 'danger' : undefined} onClick={onRun} disabled={loading}>
              {loading ? `${action.replace(/e$/, '')}ing…` : action}
            </Button>
          </Inline>
        </Stack>
      </CardContent>
    </Card>
  )
}

function describeDemoResult(r: DemoDatasetResponse): string {
  const bits: string[] = [`namespace ${r.namespace}`]
  if (r.events_created) bits.push(`${r.events_created.toLocaleString()} events created`)
  if (r.events_deleted) bits.push(`${r.events_deleted.toLocaleString()} events deleted`)
  if (r.catalog_items_created)
    bits.push(`${r.catalog_items_created.toLocaleString()} catalog items created`)
  if (r.api_key) bits.push('new api_key issued — copy from the response if you need it')
  return bits.join(' · ')
}
