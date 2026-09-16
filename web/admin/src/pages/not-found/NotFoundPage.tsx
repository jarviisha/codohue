import { useLocation } from 'react-router-dom'
import { Button, EmptyState, Stack } from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import PageHeader from '@/components/shell/PageHeader'

/**
 * NotFoundPage catches URLs that match no route.
 *
 * Without it React Router falls back to its own unstyled error screen, which
 * drops the operator out of the shell entirely — no nav, no way back except
 * the browser's back button. Rendering inside the shell keeps the sidebar and
 * breadcrumb available, so a mistyped or stale URL is a detour rather than a
 * dead end.
 */
export default function NotFoundPage() {
  const location = useLocation()

  return (
    <PageContainer size="md">
      <PageHeader>
        <Stack gap={1}>
          <h1 className="text-primary text-xl font-semibold">Not found</h1>
          <p className="text-secondary text-sm">No route matches this URL.</p>
        </Stack>
      </PageHeader>

      <Stack gap={6}>
        <EmptyState
          title="404 — no such page"
          description={`Nothing is mounted at ${location.pathname}. The link may be stale, or the namespace it pointed at may have been deleted.`}
        />
        <Stack align="center" gap={4} direction="horizontal" justify="start">
          <Button href="/" variant="secondary"  size="sm" label="Back to fleet" />
          <Button href="/namespaces" variant="ghost"  size="sm" label="Namespaces" />
        </Stack>
      </Stack>
    </PageContainer>
  )
}
