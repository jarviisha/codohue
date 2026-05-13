import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  Button,
  ConfirmDialog,
  LoadingState,
  Notice,
  PageHeader,
  PageShell,
  useRegisterCommand,
} from '../../components/ui'
import { ApiError } from '../../services/http'
import { useNamespace, useUpsertNamespace } from '../../services/namespaces'
import {
  fromNamespaceConfig,
  toUpsertPayload,
  type NamespaceFormState,
} from './configForm'
import NamespaceForm from './NamespaceForm'
import { paths } from '../../routes/path'

export default function NamespaceConfigPage() {
  const { name = '' } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const config = useNamespace(name)
  const upsert = useUpsertNamespace()

  const [state, setState] = useState<NamespaceFormState | null>(null)
  // JSON stringification is fine for dirty detection since the form state is
  // plain JSON-shaped (no Dates / Maps / class instances).
  const [pristine, setPristine] = useState<string | null>(null)
  const [showCancelConfirm, setShowCancelConfirm] = useState(false)

  // Hydrate the form once when the config arrives. Subsequent refetches do
  // not blow away in-progress edits.
  useEffect(() => {
    if (config.data && state === null) {
      const initial = fromNamespaceConfig(config.data)
      setState(initial)
      setPristine(JSON.stringify(initial))
    }
  }, [config.data, state])

  const isDirty =
    state !== null && pristine !== null && JSON.stringify(state) !== pristine

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

  if (config.isLoading || state === null) {
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

  const errorMessage =
    upsert.error instanceof ApiError
      ? upsert.error.message
      : upsert.error instanceof Error
        ? upsert.error.message
        : undefined

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
      />

      <NamespaceForm
        mode="edit"
        state={state}
        onChange={setState}
        onSubmit={handleSubmit}
        onCancel={handleCancel}
        isPending={upsert.isPending}
        errorMessage={errorMessage}
      />

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
    </PageShell>
  )
}
