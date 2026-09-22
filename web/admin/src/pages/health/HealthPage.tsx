import { Token, Skeleton, Stack, Text } from '@astryxdesign/core'
import QueryFeedback from '@/components/QueryFeedback'
import PageContainer from '@/components/PageContainer'
import { useHealth, type ComponentStatus } from '@/services/health'
import PageHeader from '@/components/shell/PageHeader'

const COMPONENTS: Array<{ key: 'postgres' | 'redis' | 'qdrant'; label: string; explain: string }> =
  [
    {
      key: 'postgres',
      label: 'PostgreSQL',
      explain: 'Events, namespace configs, batch run logs, catalog items.',
    },
    {
      key: 'redis',
      label: 'Redis',
      explain: 'Recommendation cache, trending ZSETs, ingest + embed Streams.',
    },
    { key: 'qdrant', label: 'Qdrant', explain: 'Sparse and dense vectors for recommend service.' },
  ]

function statusVariant(s: ComponentStatus): 'green' | 'orange' | 'red' | 'gray' {
  if (s.startsWith('error:')) return 'red'
  switch (s) {
    case 'ok':
      return 'green'
    case 'degraded':
      return 'orange'
    case 'error':
      return 'red'
    default:
      return 'gray'
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

  if (!health.data) {
    return (
      <PageContainer size="md">
        <PageHeader>
          <h1>Service health</h1>
        </PageHeader>
        <QueryFeedback query={health} label="Service health" />
      </PageContainer>
    )
  }

  const data = health.data!
  const overall = data.status?.trim() || 'unknown'

  return (
    <PageContainer size="md">
      <PageHeader>
        <Stack gap={1}>
          <h1 className="text-primary text-xl font-semibold">Service health</h1>
          <Stack gap={4} direction="horizontal" align="center">
            <span className="text-secondary text-sm">overall</span>
            <Token color={statusVariant(overall)} label={overall} />
            <span className="text-secondary text-xs">refreshes every 30 seconds</span>
          </Stack>
        </Stack>
      </PageHeader>

      <QueryFeedback query={health} label="Service health" />
      <Stack gap={6}>
        <Stack gap={6}>
          {COMPONENTS.map((c) => {
            const s = data[c.key]?.trim() || 'unknown'
            return (
              <Stack key={c.key} gap={2} className="border-b border-border pb-4">
                <Stack gap={1}>
                  <Stack gap={4} direction="horizontal" align="center" justify="between">
                    <Text weight="semibold">{c.label}</Text>
                    <Token color={statusVariant(s)} label={s} />
                  </Stack>
                </Stack>
                <p className="text-secondary text-sm">{c.explain}</p>
              </Stack>
            )
          })}
        </Stack>
      </Stack>
    </PageContainer>
  )
}
