import { useState } from 'react'
import {
  Button,
  ConfirmDialog,
  KeyValueList,
  Notice,
  PageHeader,
  PageShell,
  Panel,
  useRegisterCommand,
} from '@/components/ui'
import { ApiError } from '@/services/http'
import { useResetApp, type ResetAppResponse } from '@/services/danger'
import { useNamespaces } from '@/services/namespaces'
import { formatNumber } from '@/utils/format'

// Single-purpose page for app-wide destructive actions. Currently only hosts
// the full reset; future entries (e.g. wipe trending cache, drop dense
// embeddings) should land here too so the operator never has to hunt across
// the console for irreversible operations.
export default function DangerZonePage() {
  const namespaces = useNamespaces()
  const reset = useResetApp()
  const [showConfirm, setShowConfirm] = useState(false)
  const [lastResult, setLastResult] = useState<ResetAppResponse | null>(null)

  useRegisterCommand(
    'danger.resetApp',
    'Reset entire app (delete all namespaces)',
    () => setShowConfirm(true),
    'global',
  )

  const handleConfirm = () => {
    reset.mutate(undefined, {
      onSuccess: (data) => {
        setLastResult(data)
        setShowConfirm(false)
      },
    })
  }

  const errorMessage =
    reset.error instanceof ApiError
      ? reset.error.message
      : reset.error instanceof Error
        ? reset.error.message
        : undefined

  const namespaceCount = namespaces.data?.items.length ?? 0

  return (
    <PageShell>
      <PageHeader
        title="danger zone"
        meta="irreversible operations across postgres, redis, and qdrant"
      />

      <Notice tone="warn" title="These actions cannot be undone">
        Every action on this page wipes operator data across all three datastores.
        Use it only for fresh installs, demo reseeding, or recovering from a
        misconfigured deployment.
      </Notice>

      <Panel
        title="reset entire app"
        actions={
          <Button
            variant="danger"
            loading={reset.isPending}
            disabled={reset.isPending}
            onClick={() => setShowConfirm(true)}
          >
            Reset
          </Button>
        }
      >
        <div className="flex flex-col gap-3 text-sm">
          <p className="text-secondary">
            Deletes every namespace and its data: events, catalog items, batch run
            logs, id mappings, namespace configs, trending caches, recommendation
            caches, and all Qdrant collections. After running this the database
            is in the same shape as a fresh <span className="font-mono">migrate-up</span> install.
          </p>
          <p className="text-muted">
            {namespaceCount === 0
              ? 'No namespaces currently exist — this action will be a no-op.'
              : `${formatNumber(namespaceCount)} namespace${namespaceCount === 1 ? '' : 's'} will be wiped.`}
          </p>
        </div>
      </Panel>

      {errorMessage ? (
        <Notice tone="fail" title="Reset failed" onDismiss={() => reset.reset()}>
          {errorMessage}
        </Notice>
      ) : null}

      {lastResult ? (
        <Panel title="last reset">
          <KeyValueList
            rows={[
              {
                label: 'namespaces_deleted',
                value: formatNumber(lastResult.namespaces_deleted),
              },
              {
                label: 'events_deleted',
                value: formatNumber(lastResult.events_deleted),
              },
              {
                label: 'namespaces',
                value:
                  lastResult.namespaces.length > 0 ? (
                    <span className="font-mono text-primary">
                      {lastResult.namespaces.join(', ')}
                    </span>
                  ) : (
                    'none'
                  ),
              },
            ]}
          />
        </Panel>
      ) : null}

      <ConfirmDialog
        open={showConfirm}
        title="Reset entire app?"
        description="This deletes every namespace and all of its data across postgres, redis, and qdrant. There is no recovery — make sure you have backups if you need them."
        confirmLabel="Reset everything"
        destructive
        loading={reset.isPending}
        requireTyped="RESET"
        onConfirm={handleConfirm}
        onCancel={() => setShowConfirm(false)}
      />
    </PageShell>
  )
}
