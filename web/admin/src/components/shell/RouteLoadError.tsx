import { useNavigate } from 'react-router-dom'
import { Banner, Button, Stack } from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'

/**
 * RouteLoadError reports a page that could not be downloaded — the failure
 * React.lazy used to drop on the floor, leaving the shell intact around an
 * empty outlet.
 *
 * The usual cause is a deploy: the operator's tab is running the previous
 * build and asks for a hashed filename the new one no longer serves. Retrying
 * in place cannot fix that, because the browser's module registry caches the
 * failed specifier for the life of the document. Reloading is the recovery
 * that works, so it is the primary action; the fleet link is the way out for
 * an operator who would rather not reload.
 */
export default function RouteLoadError({ error }: { error?: unknown }) {
  const navigate = useNavigate()

  const detail =
    error instanceof Error
      ? error.message
      : typeof error === 'string' && error
        ? error
        : 'The page could not be downloaded.'

  return (
    <PageContainer size="md" className="py-8">
      <Stack gap={6}>
        <Banner
          status="error"
          title="Could not load this page"
          description={`${detail} This usually means the admin console was updated while your tab was open. Reloading fetches the current version.`}
        />
        <Stack align="center" gap={4} direction="horizontal" justify="end">
          <Button variant="ghost" onClick={() => navigate('/')} label="Back to fleet" />
          <Button onClick={() => window.location.reload()} label="Reload page" />
        </Stack>
      </Stack>
    </PageContainer>
  )
}
