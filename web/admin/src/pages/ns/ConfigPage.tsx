import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  Button,
  ConfirmDialog,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  Panel,
  useRegisterCommand,
} from '@/components/ui'
import { ApiError } from '@/services/http'
import {
  useDeleteNamespace,
  useNamespace,
  useUpsertNamespace,
} from '@/services/namespaces'
import type { NamespaceConfig } from '@/services/namespaces'
import {
  fromNamespaceConfig,
  toUpsertPayload,
  type NamespaceFormState,
} from './configForm'
import NamespaceForm from './namespace-form'
import { paths } from '@/routes/path'
import { clearLastNamespace } from '@/utils/lastNamespace'

const FORM_ID = 'namespace-config-form'

export default function NamespaceConfigPage() {
  const { name = '' } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const config = useNamespace(name)
  if (config.isLoading) {
    return (
      <PageShell>
        <PageHeader title="Config" meta={`namespace ${name}`} />
        <LoadingState rows={6} label="loading config" />
      </PageShell>
    )
  }

  if (config.isError || !config.data) {
    return (
      <PageShell>
        <PageHeader title="Config" meta={`namespace ${name}`} />
        <Notice tone="fail" title="Failed to load config">
          {(config.error as Error)?.message ?? 'Unknown error.'}
        </Notice>
        <div>
          <Button variant="secondary" onClick={() => navigate(paths.ns(name))}>
            Back to overview
          </Button>
        </div>
      </PageShell>
    )
  }

  return <NamespaceConfigEditor key={config.data.namespace} config={config.data} />
}

function NamespaceConfigEditor({ config }: { config: NamespaceConfig }) {
  const { name = '' } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const upsert = useUpsertNamespace()
  const deleteNs = useDeleteNamespace()

  const [state, setState] = useState<NamespaceFormState>(() =>
    fromNamespaceConfig(config),
  )
  // JSON stringification is fine for dirty detection since the form state is
  // plain JSON-shaped (no Dates / Maps / class instances).
  const [pristine, setPristine] = useState(() =>
    JSON.stringify(fromNamespaceConfig(config)),
  )
  const [showCancelConfirm, setShowCancelConfirm] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  const isDirty = JSON.stringify(state) !== pristine

  // Browser-level guard. In-app navigation guard (sidebar / topbar) requires
  // a data-router migration and lands in a later phase.
  useEffect(() => {
    if (!isDirty) return
    const handler = (e: BeforeUnloadEvent) => {
      // Modern browsers display the generic "Leave site?" prompt as long as
      // preventDefault is called; returnValue is deprecated.
      e.preventDefault()
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [isDirty])

  useRegisterCommand(
    `ns.${name}.overview`,
    `Open ${name} overview`,
    () => {
      if (isDirty) setShowCancelConfirm(true)
      else navigate(paths.ns(name))
    },
    name,
  )

  const handleSubmit = (values: NamespaceFormState) => {
    upsert.mutate(
      { name, payload: toUpsertPayload(values) },
      {
        onSuccess: () => {
          // Reset dirty marker so the unload guard releases before navigation.
          setPristine(JSON.stringify(values))
          navigate(paths.ns(name))
        },
      },
    )
  }

  const handleCancel = () => {
    if (isDirty) setShowCancelConfirm(true)
    else navigate(paths.ns(name))
  }

  const errorMessage =
    upsert.error instanceof ApiError
      ? upsert.error.message
      : upsert.error instanceof Error
        ? upsert.error.message
        : undefined

  const deleteErrorMessage =
    deleteNs.error instanceof ApiError
      ? deleteNs.error.message
      : deleteNs.error instanceof Error
        ? deleteNs.error.message
        : undefined

  const handleDelete = () => {
    deleteNs.mutate(name, {
      onSuccess: () => {
        setShowDeleteConfirm(false)
        // Release the unsaved-changes guard before leaving the page.
        setPristine(JSON.stringify(state))
        // Drop the sticky sidebar memory so the namespace nav doesn't
        // linger pointing at a namespace that no longer exists.
        clearLastNamespace()
        navigate(paths.namespaces)
      },
    })
  }

  return (
    <PageShell>
      <PageHeader
        title="Config"
        meta={
          <span>
            namespace <span className="text-primary">{name}</span>
            {isDirty ? (
              <span className="text-warning ml-2">· unsaved changes</span>
            ) : null}
          </span>
        }
        actions={
          <>
            <Button variant="ghost" size="sm" onClick={handleCancel}>
              Cancel
            </Button>
            <Button
              type="submit"
              form={FORM_ID}
              variant="primary"
              size="sm"
              loading={upsert.isPending}
            >
              Save
            </Button>
          </>
        }
      />

      <NamespaceForm
        mode="edit"
        formId={FORM_ID}
        state={state}
        onChange={setState}
        onSubmit={handleSubmit}
        isPending={upsert.isPending}
        errorMessage={errorMessage}
      />

      <Panel
        title="danger zone"
        actions={
          <Button
            variant="danger"
            size="sm"
            loading={deleteNs.isPending}
            disabled={deleteNs.isPending}
            onClick={() => setShowDeleteConfirm(true)}
          >
            Delete namespace
          </Button>
        }
      >
        <div className="flex flex-col gap-3 text-sm">
          <p className="text-secondary">
            Permanently removes namespace{' '}
            <span className="font-mono text-primary">{name}</span> and every trace
            of its data: events, catalog items, batch run logs, id mappings,
            trending caches, recommendation caches, and Qdrant collections.
          </p>
          <p className="text-muted">This cannot be undone.</p>
        </div>
        {deleteErrorMessage ? (
          <div className="mt-3">
            <Notice tone="fail" title="Delete failed" onDismiss={() => deleteNs.reset()}>
              {deleteErrorMessage}
            </Notice>
          </div>
        ) : null}
      </Panel>

      <ConfirmDialog
        open={showCancelConfirm}
        title="Discard unsaved changes?"
        description="Your form edits will be lost. Are you sure you want to leave?"
        confirmLabel="Discard"
        cancelLabel="Stay"
        destructive
        onConfirm={() => {
          setShowCancelConfirm(false)
          navigate(paths.ns(name))
        }}
        onCancel={() => setShowCancelConfirm(false)}
      />

      <ConfirmDialog
        open={showDeleteConfirm}
        title={`Delete namespace ${name}?`}
        description="This removes the namespace from postgres, redis, and qdrant. There is no recovery."
        confirmLabel="Delete namespace"
        destructive
        loading={deleteNs.isPending}
        requireTyped={name}
        onConfirm={handleDelete}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </PageShell>
  )
}
