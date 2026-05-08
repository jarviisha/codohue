import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useNamespace, useUpsertNamespace } from '../hooks/useNamespaces'
import { useQdrant } from '../hooks/useQdrant'
import ErrorBanner from '../components/ErrorBanner'
import { LoadingState, PageHeader, PageShell } from '../components/ui'
import {
  defaultNamespaceForm,
  namespaceConfigToForm,
  namespaceFormToPayload,
  type NamespaceFormState,
} from './namespaceForm'
import CreatedApiKeyPanel from './namespace-detail/CreatedApiKeyPanel'
import NamespaceForm from './namespace-detail/NamespaceForm'
import QdrantStatsPanel from './namespace-detail/QdrantStatsPanel'

export default function NamespaceDetailPage() {
  const { ns } = useParams<{ ns: string }>()
  const isNew = !ns || ns === 'new'
  const navigate = useNavigate()

  const { data: existing, error: loadErr, isLoading } = useNamespace(ns ?? '')
  const { data: qdrantStats } = useQdrant(isNew ? '' : (ns ?? ''))
  const upsert = useUpsertNamespace()

  const [newKey, setNewKey] = useState<string | null>(null)
  const [saveError, setSaveError] = useState('')

  async function handleSave(form: NamespaceFormState) {
    setSaveError('')
    const nsName = isNew ? form.name : ns!
    try {
      const result = await upsert.mutateAsync({
        ns: nsName,
        payload: namespaceFormToPayload(form),
      })
      if (result.api_key) {
        setNewKey(result.api_key)
      } else {
        navigate('/namespaces')
      }
    } catch (err: unknown) {
      setSaveError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  if (loadErr) return <ErrorBanner message="Failed to load namespace config." />

  const initialForm = isNew
    ? defaultNamespaceForm()
    : existing
      ? namespaceConfigToForm(existing)
      : null

  return (
    <PageShell>
      <PageHeader title={isNew ? 'Create Namespace' : `Namespace Settings: ${ns}`} />

      {newKey && (
        <CreatedApiKeyPanel apiKey={newKey} onDone={() => navigate('/namespaces')} />
      )}

      {saveError && <ErrorBanner message={saveError} onDismiss={() => setSaveError('')} />}

      {!isNew && qdrantStats && (
        <QdrantStatsPanel stats={qdrantStats} />
      )}

      {!newKey && isLoading && !initialForm && (
        <LoadingState label="Loading namespace config..." />
      )}

      {!newKey && initialForm && (
        <NamespaceForm
          key={isNew ? 'new' : existing?.updated_at ?? ns}
          initialForm={initialForm}
          isNew={isNew}
          isPending={upsert.isPending}
          onCancel={() => navigate('/namespaces')}
          onSubmit={handleSave}
        />
      )}
    </PageShell>
  )
}
