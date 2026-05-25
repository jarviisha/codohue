import {
  Alert,
  Badge,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Container,
  Inline,
  Skeleton,
  Stack,
} from '@jarviisha/davinci-react-ui'
import { useHealth, type ComponentStatus } from '@/services/health'
import PageHeader from '@/components/shell/PageHeader'

const COMPONENTS: Array<{ key: 'postgres' | 'redis' | 'qdrant'; label: string; explain: string }> = [
  { key: 'postgres', label: 'PostgreSQL', explain: 'Events, namespace configs, batch run logs, catalog items.' },
  { key: 'redis', label: 'Redis', explain: 'Recommendation cache, trending ZSETs, ingest + embed Streams.' },
  { key: 'qdrant', label: 'Qdrant', explain: 'Sparse and dense vectors for recommend service.' },
]

function statusVariant(s: ComponentStatus): 'success' | 'warning' | 'danger' | 'neutral' {
  switch (s) {
    case 'ok':
      return 'success'
    case 'degraded':
      return 'warning'
    case 'error':
      return 'danger'
    default:
      return 'neutral'
  }
}

export default function HealthPage() {
  const health = useHealth()

  if (health.isLoading) {
    return (
      <Container size="md" className="py-6">
        <Skeleton className="h-48 w-full" />
      </Container>
    )
  }

  if (health.isError) {
    return (
      <Container size="md" className="py-6">
        <Alert
          variant="danger"
          title="Could not reach /health"
          description={health.error?.message ?? 'unknown error'}
        />
      </Container>
    )
  }

  const data = health.data!

  return (
    <Container size="md" className="py-6">
      <PageHeader>
        <Stack gap="025">
          <h1 className="text-foreground text-xl font-semibold">Service health</h1>
          <Inline gap="100" align="center">
            <span className="text-foreground-subtle text-sm">overall</span>
            <Badge variant={statusVariant(data.status)}>{data.status}</Badge>
            <span className="text-foreground-subtle text-xs">refreshes every 30 seconds</span>
          </Inline>
        </Stack>
      </PageHeader>

      <Stack gap="300">
        <Stack gap="200">
          {COMPONENTS.map((c) => {
            const s = data[c.key]
            return (
              <Card key={c.key}>
                <CardHeader>
                  <Inline gap="100" align="center" justify="between">
                    <CardTitle>{c.label}</CardTitle>
                    <Badge variant={statusVariant(s)}>{s}</Badge>
                  </Inline>
                </CardHeader>
                <CardContent>
                  <p className="text-foreground-subtle text-sm">{c.explain}</p>
                </CardContent>
              </Card>
            )
          })}
        </Stack>
      </Stack>
    </Container>
  )
}
