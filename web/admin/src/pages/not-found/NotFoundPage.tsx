import { useLocation } from 'react-router-dom'
import { Container, EmptyState, Inline, Stack } from '@jarviisha/davinci-react-ui'
import PageHeader from '@/components/shell/PageHeader'
import LinkButton from '@/components/LinkButton'

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
    <Container size="md" className="py-6 px-6">
      <PageHeader>
        <Stack gap="050">
          <h1 className="text-foreground text-xl font-semibold">Not found</h1>
          <p className="text-foreground-subtle text-sm">No route matches this URL.</p>
        </Stack>
      </PageHeader>

      <Stack>
        <EmptyState
          title="404 — no such page"
          description={`Nothing is mounted at ${location.pathname}. The link may be stale, or the namespace it pointed at may have been deleted.`}
        />
        <Inline justify="start">
          <LinkButton to="/" variant="outline" tone="neutral" size="sm">
            Back to fleet
          </LinkButton>
          <LinkButton to="/namespaces" variant="ghost" tone="neutral" size="sm">
            Namespaces
          </LinkButton>
        </Inline>
      </Stack>
    </Container>
  )
}
