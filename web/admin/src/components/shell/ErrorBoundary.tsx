import { Component, type ErrorInfo, type ReactNode } from 'react'
import {
  Alert,
  Button,
  Container,
  Inline,
  Stack,
} from '@jarviisha/davinci-react-ui'

type Props = {
  children: ReactNode
  /** Stable key (e.g. route pathname) — when it changes the boundary resets. */
  resetKey?: string
}

type State = {
  error: Error | null
  componentStack: string | null
}

/**
 * RouteErrorBoundary catches render-time errors inside the AppShell outlet so
 * a broken page renders an Alert instead of crashing the whole SPA.
 *
 * It resets whenever `resetKey` changes (pinned to the route pathname in
 * AppShellLayout) so navigating away from a broken page recovers cleanly
 * without a page reload. Errors are mirrored to console.error to keep the
 * usual devtools workflow.
 */
export default class RouteErrorBoundary extends Component<Props, State> {
  state: State = { error: null, componentStack: null }

  static getDerivedStateFromError(error: Error): State {
    return { error, componentStack: null }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[admin] route error boundary caught:', error, info.componentStack)
    this.setState({ componentStack: info.componentStack ?? null })
  }

  componentDidUpdate(prevProps: Props) {
    if (this.state.error && prevProps.resetKey !== this.props.resetKey) {
      this.setState({ error: null, componentStack: null })
    }
  }

  private handleReload = () => {
    window.location.reload()
  }

  private handleHome = () => {
    window.location.assign('/')
  }

  render() {
    if (!this.state.error) return this.props.children

    return (
      <Container size="md" className="py-8">
        <Stack>
          <Alert
            variant="danger"
            title="This page crashed"
            description={this.state.error.message || 'An unexpected error occurred.'}
          />
          {this.state.componentStack && (
            <details>
              <summary className="text-foreground-subtle text-xs cursor-pointer">
                Component stack
              </summary>
              <pre className="text-foreground-subtle text-xs whitespace-pre-wrap mt-2">
                {this.state.componentStack.trim()}
              </pre>
            </details>
          )}
          <Inline justify="end">
            <Button variant="ghost" onClick={this.handleHome}>
              Back to fleet
            </Button>
            <Button onClick={this.handleReload}>Reload page</Button>
          </Inline>
        </Stack>
      </Container>
    )
  }
}
