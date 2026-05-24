import { useState } from 'react'
import {
  Alert,
  Badge,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Container,
  Inline,
  Stack,
} from '@jarviisha/davinci-react-ui'
import { useServerStream } from '@/services/stream'

/**
 * Phase 0 placeholder. Renders inside AppShellMain to prove that the shell,
 * routing, auth probe, theme switching, and SSE pipeline all work end-to-end.
 * Real Fleet overview replaces this in Phase 1.
 */
export default function HomePage() {
  const [lastTickAt, setLastTickAt] = useState<string | null>(null)
  const [tickCount, setTickCount] = useState(0)
  const [errored, setErrored] = useState(false)

  const { connected } = useServerStream(
    '/api/admin/v1/ping/stream',
    {
      tick: (payload) => {
        const at = (payload as { at?: string })?.at ?? new Date().toISOString()
        setLastTickAt(at)
        setTickCount((n) => n + 1)
      },
    },
    {
      onError: () => setErrored(true),
    },
  )

  return (
    <Container size="md" className="py-8">
      <Stack gap="300">
        <h1 className="text-foreground text-2xl font-semibold">codohue admin · phase 0</h1>
        <Alert
          variant="info"
          title="Skeleton scaffold"
          description="App shell + auth + theme + SSE smoke endpoint are wired. Phase 1 lands the real Fleet overview."
        />
        <Card>
          <CardHeader>
            <CardTitle>SSE pipeline smoke</CardTitle>
            <CardDescription>
              Live stream from <code>GET /api/admin/v1/ping/stream</code>. Tick once per second.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Stack gap="200">
              <Inline gap="100" align="center">
                <Badge variant={connected ? 'success' : errored ? 'danger' : 'neutral'}>
                  {connected ? 'connected' : errored ? 'error' : 'connecting'}
                </Badge>
                <span className="text-foreground-subtle text-sm">
                  ticks received: <strong className="text-foreground">{tickCount}</strong>
                </span>
              </Inline>
              <p className="text-foreground-subtle text-sm">
                last tick: <code>{lastTickAt ?? '—'}</code>
              </p>
            </Stack>
          </CardContent>
        </Card>
      </Stack>
    </Container>
  )
}
