import {
  Banner,
  Heading,
  Skeleton,
  Stack,
  Table,
  TableHeader,
  TableRow,
  TableHeaderCell,
  TableBody,
  TableCell,
  Token,
  proportional,
} from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import PageHeader from '@/components/shell/PageHeader'
import QueryFeedback from '@/components/QueryFeedback'
import { useRuntime } from '@/services/runtime'

export default function RuntimePage() {
  const runtime = useRuntime()
  return (
    <PageContainer size="lg">
      <PageHeader>
        <Stack gap={2}>
          <h1 className="text-primary text-xl font-semibold">System runtime</h1>
          <p className="text-secondary text-sm">
            Read-only effective settings reported by each running process.
          </p>
        </Stack>
      </PageHeader>
      <Stack gap={6}>
        <Banner
          status="info"
          title="Managed by deployment"
          description="These values were loaded at process startup. Change the deployment configuration and restart the affected process to apply changes. Credentials and connection strings are never displayed. Namespace overrides remain in Configuration."
        />
        <QueryFeedback query={runtime} label="Runtime reports" />
        {runtime.isLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : (
          runtime.data && (
            <>
              <p className="text-secondary text-sm">
                Processes report every 30 seconds. Reports expire after{' '}
                {runtime.data.expiry_seconds} seconds. A recent report confirms reporting activity,
                not dependency health. Missing reports do not prove a process is stopped.
              </p>
              {['api', 'admin', 'cron', 'embedder'].map((process) => {
                const reports = runtime.data.processes.filter(
                  (report) => report.process === process,
                )
                return (
                  <Stack key={process} gap={3}>
                    <Heading level={2}>{process}</Heading>
                    {!reports.length && <Token color="gray" label="No recent report" />}
                    {reports.map((report) => (
                      <Stack key={`${report.instance}-${report.started_at}`} gap={3}>
                        <p className="text-secondary text-sm">
                          Instance {report.instance} · Started{' '}
                          {new Date(report.started_at).toLocaleString()} · Reported{' '}
                          {new Date(report.reported_at).toLocaleString()}
                        </p>
                        <Table
                          columns={['Setting', 'Effective value'].map((key) => ({
                            key,
                            label: key,
                            width: proportional(1),
                          }))}
                        >
                          <TableHeader>
                            <TableRow>
                              <TableHeaderCell>Setting</TableHeaderCell>
                              <TableHeaderCell>Effective value</TableHeaderCell>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {report.settings.map((setting) => (
                              <TableRow key={setting.name}>
                                <TableCell>{setting.name.replaceAll('_', ' ')}</TableCell>
                                <TableCell>
                                  {typeof setting.value === 'boolean'
                                    ? setting.value
                                      ? 'Yes'
                                      : 'No'
                                    : String(setting.value)}
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </Stack>
                    ))}
                  </Stack>
                )
              })}
            </>
          )
        )}
      </Stack>
    </PageContainer>
  )
}
