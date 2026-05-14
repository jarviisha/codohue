import { Outlet, useParams } from 'react-router-dom'
import {
  PageHeader,
  PageShell,
  TabLink,
  Tabs,
} from '@/components/ui'
import { paths } from '@/routes/path'

// Shared layout for /ns/:name/batch-runs. Two tabs:
//   - CF runs (sparse + dense + trending, both cron and admin-triggered)
//   - Re-embeds (catalog re-embed orchestration)
// Each tab owns its own list query — the layout only draws the chrome.
export default function BatchRunsLayout() {
  const { name = '' } = useParams<{ name: string }>()

  return (
    <PageShell>
      <PageHeader title="Batch runs" meta={`namespace ${name}`} />

      <Tabs ariaLabel="Batch run kind">
        <TabLink to={paths.nsBatchRuns(name)} end>CF runs</TabLink>
        <TabLink to={paths.nsBatchRunsReEmbeds(name)}>Re-embeds</TabLink>
      </Tabs>

      <Outlet />
    </PageShell>
  )
}
