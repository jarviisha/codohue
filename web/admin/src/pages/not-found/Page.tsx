import { Link, useLocation, useParams } from 'react-router-dom'
import {
  Button,
  KeyValueList,
  PageHeader,
  PageShell,
  Panel,
} from '@/components/ui'
import { paths } from '@/routes/path'

// Catch-all destination for unknown URLs. Renders inside AppShell so the
// sidebar + PS1 prompt stay anchored; the body echoes the bad path so the
// operator can confirm what they typed.
//
// The same component handles both the top-level `*` and the nested `*`
// inside `ns/:name`. When the namespace param is present, "back to overview"
// links into that namespace; otherwise it falls back to the global Health
// page.
export default function NotFoundPage() {
  const { pathname, search } = useLocation()
  const { name } = useParams<{ name?: string }>()
  const fullPath = pathname + search

  return (
    <PageShell>
      <PageHeader title="not found" />

      <Panel title="unknown path">
        <div className="flex flex-col gap-4">
          <p className="text-sm text-secondary">
            The path below did not match any route. Use the sidebar or the actions
            below to get back to a known view.
          </p>

          <KeyValueList
            rows={[
              { label: 'path', value: <span className="break-all">{fullPath}</span> },
              ...(name
                ? [{ label: 'namespace', value: name }]
                : []),
            ]}
          />

          <div className="flex flex-wrap items-center gap-2">
            {name ? (
              <Link to={paths.ns(name)}>
                <Button variant="primary">Back to {name}</Button>
              </Link>
            ) : null}
            <Link to={paths.health}>
              <Button variant={name ? 'secondary' : 'primary'}>Go to health</Button>
            </Link>
            <Link to={paths.namespaces}>
              <Button variant="secondary">Go to namespaces</Button>
            </Link>
          </div>
        </div>
      </Panel>
    </PageShell>
  )
}
