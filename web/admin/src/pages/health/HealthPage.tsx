import { Badge, Banner, Card, Skeleton, Stack, Text } from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import { useHealth, type ComponentStatus } from '@/services/health'
import PageHeader from '@/components/shell/PageHeader'

const COMPONENTS: Array<{ key: 'postgres' | 'redis' | 'qdrant'; label: string; explain: string }> = [
  { key: 'postgres', label: 'PostgreSQL', explain: 'Events, namespace configs, batch run logs, catalog items.' },
  { key: 'redis', label: 'Redis', explain: 'Recommendation cache, trending ZSETs, ingest + embed Streams.' },
  { key: 'qdrant', label: 'Qdrant', explain: 'Sparse and dense vectors for recommend service.' },
]

function statusVariant(s: ComponentStatus): 'success' | 'warning' | 'error' | 'neutral' {
  switch (s) {
    case 'ok':
      return 'success'
    case 'degraded':
      return 'warning'
    case 'error':
      return 'error'
    default:
      return 'neutral'
  }
}

export default function HealthPage() {
  const health = useHealth()

  if (health.isLoading) {
    return (
      <PageContainer size="md">
        <Skeleton className="h-48 w-full" />
      </PageContainer>
    )
  }

  if (health.isError) {
    return (
      <PageContainer size="md">
        <Banner
          status="error"
          title="Could not reach /health"
          description={health.error?.message ?? 'unknown error'}
        />
      </PageContainer>
    )
  }

  const data = health.data!

  return (
    <PageContainer size="md">
      <PageHeader>
        <Stack gap={1}>
          <h1 className="text-primary text-xl font-semibold">Service health</h1>
          <Stack gap={4} direction="horizontal" align="center">
            <span className="text-secondary text-sm">overall</span>
            <Badge variant={statusVariant(data.status)} label={data.status} />
            <span className="text-secondary text-xs">refreshes every 30 seconds</span>
          </Stack>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        <Stack gap={6}>
          {COMPONENTS.map((c) => {
            const s = data[c.key]
            return (
              <Card key={c.key}>
                <Stack gap={1}>
                  <Stack gap={4} direction="horizontal" align="center" justify="between">
                    <Text weight="semibold">{c.label}</Text>
                    <Badge variant={statusVariant(s)} label={s} />
                  </Stack>
                </Stack>
                  <p className="text-secondary text-sm">{c.explain}</p>
              </Card>
            )
          })}
        </Stack>
      </Stack>
    </PageContainer>
  )
}
