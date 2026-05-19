import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  CodeBlock,
  ConfirmDialog,
  KeyValueList,
  Notice,
  PageHeader,
  PageShell,
  Panel,
  useRegisterCommand,
} from '@/components/ui'
import { http } from '@/services/http'
import { namespaceKeys } from '@/services/namespaces'
import { paths } from '@/routes/path'
import { formatNumber } from '@/utils/format'

// Two-endpoint surface (seed + clear) — the request functions live inline
// rather than getting their own services/demoData.ts.

interface DemoDatasetResponse {
  namespace: string
  events_created?: number
  events_deleted?: number
  catalog_items_created?: number
  // Only populated by the seed action when a fresh API key was generated.
  api_key?: string
}

function seedDemoData() {
  return http.post<DemoDatasetResponse>('/api/admin/v1/demo-data')
}

// The endpoint returns 204; we synthesise a sentinel so the UI can still
// react to the success and refresh namespace data.
function clearDemoData(): Promise<DemoDatasetResponse> {
  return http
    .del<undefined>('/api/admin/v1/demo-data')
    .then(() => ({ namespace: 'demo' }))
}

// Seeds (or wipes) the bundled "demo" namespace + events + catalog items so
// fresh installs have realistic data to explore the rest of the console.
export default function DemoDataPage() {
  const qc = useQueryClient()
  const [showSeedConfirm, setShowSeedConfirm] = useState(false)
  const [showClearConfirm, setShowClearConfirm] = useState(false)
  const [lastAction, setLastAction] = useState<'seed' | 'clear' | null>(null)
  const [lastResult, setLastResult] = useState<DemoDatasetResponse | null>(null)

  const onMutationSettled = () => {
    qc.invalidateQueries({ queryKey: namespaceKeys.all })
  }

  const seed = useMutation({
    mutationFn: seedDemoData,
    onSuccess: (data) => {
      setLastAction('seed')
      setLastResult(data)
      setShowSeedConfirm(false)
    },
    onSettled: onMutationSettled,
  })

  const clear = useMutation({
    mutationFn: clearDemoData,
    onSuccess: (data) => {
      setLastAction('clear')
      setLastResult(data)
      setShowClearConfirm(false)
    },
    onSettled: onMutationSettled,
  })

  useRegisterCommand(
    'admin.demoData.seed',
    'Seed bundled demo dataset',
    () => setShowSeedConfirm(true),
  )
  useRegisterCommand(
    'admin.demoData.clear',
    'Clear bundled demo dataset',
    () => setShowClearConfirm(true),
  )

  const demoNamespace = lastResult?.namespace ?? 'demo'
  const isBusy = seed.isPending || clear.isPending

  return (
    <PageShell>
      <PageHeader title="demo data" />

      <Panel
        title="bundled dataset"
        actions={
          <>
            <Button
              variant="primary"
              loading={seed.isPending}
              disabled={isBusy}
              onClick={() => setShowSeedConfirm(true)}
            >
              Seed
            </Button>
            <Button
              variant="danger"
              loading={clear.isPending}
              disabled={isBusy}
              onClick={() => setShowClearConfirm(true)}
            >
              Clear
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-3 text-sm">
          <p className="text-secondary">
            Seeds the <span className="font-mono text-primary">demo</span> namespace with sample
            events and catalog items so fresh installs have realistic data to explore. Clearing
            removes all rows the seed inserted across postgres, redis, and qdrant.
          </p>
        </div>
      </Panel>

      {seed.isError ? (
        <Notice tone="fail" title="Seed failed" onDismiss={() => seed.reset()}>
          {(seed.error as Error)?.message ?? 'Unable to seed demo dataset.'}
        </Notice>
      ) : null}
      {clear.isError ? (
        <Notice tone="fail" title="Clear failed" onDismiss={() => clear.reset()}>
          {(clear.error as Error)?.message ?? 'Unable to clear demo dataset.'}
        </Notice>
      ) : null}

      {lastAction === 'seed' && lastResult ? (
        <Notice tone="ok" title="Demo dataset seeded" onDismiss={() => setLastResult(null)}>
          Namespace{' '}
          <Link
            to={paths.ns(lastResult.namespace)}
            className="font-mono text-accent hover:underline"
          >
            {lastResult.namespace}
          </Link>{' '}
          is ready. Open the overview to start exploring.
        </Notice>
      ) : null}
      {lastAction === 'clear' && lastResult ? (
        <Notice tone="ok" title="Demo dataset cleared" onDismiss={() => setLastResult(null)}>
          All demo rows removed across postgres, redis, and qdrant.
        </Notice>
      ) : null}

      {lastResult && lastAction === 'seed' ? (
        <Panel title="last run">
          <KeyValueList
            rows={[
              { label: 'action', value: 'seed' },
              { label: 'namespace', value: lastResult.namespace },
              {
                label: 'events_created',
                value: formatNumber(lastResult.events_created ?? 0),
              },
              {
                label: 'catalog_items_created',
                value: formatNumber(lastResult.catalog_items_created ?? 0),
              },
              {
                label: 'api_key',
                value: lastResult.api_key ? (
                  <span title="Visible once — copy it now">{lastResult.api_key}</span>
                ) : (
                  'unchanged'
                ),
              },
            ]}
          />
          {lastResult.api_key ? (
            <div className="mt-3">
              <CodeBlock language="text" copyable>{lastResult.api_key}</CodeBlock>
              <p className="mt-2 text-xs text-warning">
                Copy this key now — it will not be shown again.
              </p>
            </div>
          ) : null}
        </Panel>
      ) : null}

      {lastResult && lastAction === 'clear' ? (
        <Panel title="last run">
          <KeyValueList
            rows={[
              { label: 'action', value: 'clear' },
              { label: 'namespace', value: lastResult.namespace },
              {
                label: 'events_deleted',
                value: formatNumber(lastResult.events_deleted ?? 0),
              },
            ]}
          />
        </Panel>
      ) : null}

      <ConfirmDialog
        open={showSeedConfirm}
        title="Seed bundled demo dataset?"
        description={`Creates namespace "${demoNamespace}" if missing, then inserts the bundled events and catalog items. Safe to re-run.`}
        confirmLabel="Seed"
        loading={seed.isPending}
        onConfirm={() => seed.mutate()}
        onCancel={() => setShowSeedConfirm(false)}
      />

      <ConfirmDialog
        open={showClearConfirm}
        title="Clear bundled demo dataset?"
        description={`Removes every demo row from namespace "${demoNamespace}" across postgres, redis, and qdrant. This cannot be undone.`}
        confirmLabel="Clear"
        destructive
        loading={clear.isPending}
        onConfirm={() => clear.mutate()}
        onCancel={() => setShowClearConfirm(false)}
      />
    </PageShell>
  )
}
