import { Component, type ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import {
  Button,
  CodeBlock,
  Notice,
  PageHeader,
  PageShell,
  Panel,
} from '@/components/ui'

interface ErrorBoundaryProps {
  children: ReactNode
}

interface ErrorBoundaryState {
  error: Error | null
}

// Class component — React still requires class-based error boundaries.
// Wrapped by `RouteErrorBoundary` below, which uses the route pathname as a
// key so navigating away from a crashed page resets the boundary
// automatically.
class ErrorBoundaryImpl extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // Surface to the browser console so devs see the full stack in DevTools.
    // Reporting to an external sink is out of scope for Phase 3.
    console.error('AppShell error boundary caught:', error, info)
  }

  reset = () => this.setState({ error: null })

  render() {
    const { error } = this.state
    if (!error) return this.props.children

    return (
      <PageShell>
        <PageHeader title="page crashed" />

        <Notice tone="fail" title="Something went wrong">
          {error.message || 'An unexpected error occurred while rendering this page.'}
        </Notice>

        <Panel
          title="recovery"
          actions={
            <>
              <Button variant="secondary" onClick={this.reset}>
                Try again
              </Button>
              <Button variant="primary" onClick={() => window.location.reload()}>
                Reload
              </Button>
            </>
          }
        >
          <p className="text-sm text-secondary">
            "Try again" re-renders this view in case the error was transient. "Reload"
            does a hard refresh of the SPA — pick that if "try again" loops back here.
          </p>
        </Panel>

        {import.meta.env.DEV && error.stack ? (
          <Panel title="stack (dev only)">
            <CodeBlock language="text" copyable maxHeight="20rem">
              {error.stack}
            </CodeBlock>
          </Panel>
        ) : null}
      </PageShell>
    )
  }
}

// Wraps the class boundary with a route-change reset: keying on pathname
// forces React to remount the boundary when the operator navigates away
// from a crashed page, so the sidebar still works as an escape hatch.
export default function ErrorBoundary({ children }: ErrorBoundaryProps) {
  const { pathname } = useLocation()
  return <ErrorBoundaryImpl key={pathname}>{children}</ErrorBoundaryImpl>
}
