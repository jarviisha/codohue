import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Button,
  CodeBadge,
  ConfirmDialog,
  KeyValueList,
  KeyValueRow,
  Notice,
  Panel,
} from '../../components/ui'
import { useActiveNamespace } from '../../context/useActiveNamespace'
import { useClearDemoDataset, useSeedDemoDataset } from '../../hooks/useDemoDataset'
import { useNamespacesOverview } from '../../hooks/useNamespacesOverview'

const DEMO_NAMESPACE = 'demo'

export default function DemoDataPanel({ namespace }: { namespace: string }) {
  const navigate = useNavigate()
  const { setNamespace } = useActiveNamespace()
  const { data } = useNamespacesOverview()
  const seedDemo = useSeedDemoDataset()
  const clearDemo = useClearDemoDataset()
  const [confirmClear, setConfirmClear] = useState(false)

  const demoExists = data?.items.some(item => item.config.namespace === DEMO_NAMESPACE) ?? false
  const isDemoSettings = namespace === DEMO_NAMESPACE

  function openDemo(path: 'overview' | 'settings') {
    setNamespace(DEMO_NAMESPACE)
    navigate(`/namespaces/${DEMO_NAMESPACE}/${path}`)
  }

  function handleSeedDemo() {
    seedDemo.mutate()
  }

  function handleClearDemo() {
    clearDemo.mutate(undefined, {
      onSuccess: () => {
        setConfirmClear(false)
        if (isDemoSettings) navigate('/namespaces')
      },
    })
  }

  return (
    <Panel title="Demo Data">
      <div className="flex flex-col gap-4">
        <Notice tone={isDemoSettings ? 'accent' : 'warning'} role="status">
          <span className="block font-medium text-primary">
            Bundled demo data currently targets <CodeBadge>demo</CodeBadge> only.
          </span>
          <span className="mt-1 block text-sm">
            This action does not seed data into <CodeBadge>{namespace}</CodeBadge>. Supporting multiple demo datasets or a target namespace requires a namespace-scoped demo endpoint.
          </span>
        </Notice>

        <KeyValueList>
          <KeyValueRow label="Dataset" value="Bundled feed personalization sample" />
          <KeyValueRow label="Target namespace" value={<CodeBadge>{DEMO_NAMESPACE}</CodeBadge>} />
          <KeyValueRow label="Reset action" value="Creates or updates demo config, replaces demo events/catalog items, and enables catalog auto-embedding." />
          <KeyValueRow label="Clear action" value="Removes demo data from Postgres, Redis cache/streams, and Qdrant demo collections." />
        </KeyValueList>

        {seedDemo.data && (
          <Notice tone="success" role="status">
            <span className="block font-medium text-primary">Demo dataset ready.</span>
            <span className="mt-1 block text-sm">
              Seeded {seedDemo.data.events_created ?? 0} events
              {seedDemo.data.catalog_items_created != null
                ? ` and ${seedDemo.data.catalog_items_created} catalog items`
                : ''}
              .
            </span>
            {seedDemo.data.api_key && (
              <span className="mt-2 block break-all font-mono text-xs text-secondary">
                Namespace key: {seedDemo.data.api_key}
              </span>
            )}
          </Notice>
        )}

        {clearDemo.data && (
          <Notice tone="success" role="status">
            Cleared {clearDemo.data.events_deleted ?? 0} demo events.
          </Notice>
        )}

        {seedDemo.isError && (
          <Notice tone="danger" role="alert">
            Failed to seed demo dataset.
          </Notice>
        )}
        {clearDemo.isError && (
          <Notice tone="danger" role="alert">
            Failed to clear demo dataset.
          </Notice>
        )}

        <div className="flex flex-wrap gap-2 border-t border-default pt-4">
          <Button
            type="button"
            variant="primary"
            disabled={seedDemo.isPending || clearDemo.isPending}
            onClick={handleSeedDemo}
          >
            {seedDemo.isPending ? 'Resetting demo...' : demoExists ? 'Reset demo dataset' : 'Create demo dataset'}
          </Button>
          <Button
            type="button"
            variant="danger"
            disabled={!demoExists || seedDemo.isPending || clearDemo.isPending}
            onClick={() => setConfirmClear(true)}
          >
            Clear demo data
          </Button>
          <Button type="button" variant="ghost" disabled={!demoExists} onClick={() => openDemo('overview')}>
            Open demo overview
          </Button>
          <Button type="button" variant="ghost" disabled={!demoExists} onClick={() => openDemo('settings')}>
            Open demo settings
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={confirmClear}
        title="Clear demo data"
        confirmLabel="Clear demo"
        tone="danger"
        isPending={clearDemo.isPending}
        onCancel={() => setConfirmClear(false)}
        onConfirm={handleClearDemo}
      >
        <p className="m-0">
          This clears the bundled <CodeBadge>demo</CodeBadge> namespace data from Postgres, Redis, and Qdrant. It does not affect namespace <CodeBadge>{namespace}</CodeBadge>.
        </p>
      </ConfirmDialog>
    </Panel>
  )
}
