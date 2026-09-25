import { Layout, List, ListItem, Skeleton, Stack, StatusDot, Token } from '@astryxdesign/core'
import QueryFeedback from '@/components/QueryFeedback'
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

function statusVariant(s: ComponentStatus): 'success' | 'warning' | 'error' | 'neutral' {
  if (s.startsWith('error')) return 'error'
  switch (s) {
    case 'ok':
      return 'success'
    case 'degraded':
      return 'warning'
    default:
      return 'neutral'
  }
}

const TOKEN_COLOR: Record<ReturnType<typeof statusVariant>, 'green' | 'orange' | 'red' | 'gray'> = {
  success: 'green',
  warning: 'orange',
  error: 'red',
  neutral: 'gray',
}

export default function HealthPage() {
  const health = useHealth()

  if (health.isLoading) {
    return (
      <Layout height="auto" contentWidth={896}>
        <Skeleton height={192} />
      </Layout>
    )
  }

  if (!health.data) {
    return (
      <Layout height="auto" contentWidth={896}>
        <PageHeader>
          <h1>Service health</h1>
        </PageHeader>
        <QueryFeedback query={health} label="Service health" />
      </Layout>
    )
  }

  const data = health.data!
  const overall = data.status?.trim() || 'unknown'

  return (
    <Layout height="auto" contentWidth={896}>
      <PageHeader>
        <Stack gap={1}>
          <h1 className="text-primary text-xl font-semibold">Service health</h1>
          <Stack gap={4} direction="horizontal" align="center">
            <span className="text-secondary text-sm">overall</span>
            <Token color={TOKEN_COLOR[statusVariant(overall)]} label={overall} />
            <span className="text-secondary text-xs">refreshes every 30 seconds</span>
          </Stack>
        </Stack>
      </PageHeader>

      <QueryFeedback query={health} label="Service health" />
      <List hasDividers>
        {COMPONENTS.map((c) => {
          const s = data[c.key]?.trim() || 'unknown'
          return (
            <ListItem
              key={c.key}
              label={c.label}
              description={c.explain}
              endContent={
                <Stack gap={2} direction="horizontal" align="center">
                  <StatusDot variant={statusVariant(s)} label={s} />
                  <span aria-hidden="true" className="text-secondary text-sm">
                    {s}
                  </span>
                </Stack>
              }
            />
          )
        })}
      </List>
    </Layout>
  )
}
