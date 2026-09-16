import { useMemo, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Banner,
  Button,
  Dialog,
  DialogHeader,
  Layout,
  LayoutContent,
  LayoutFooter,
  NumberInput,
  Selector,
  Stack,
  TextInput,
} from '@astryxdesign/core'
import { lookupNamespace, useUpsertNamespace } from '@/services/namespaces'
import { useCatalogStrategies } from '@/services/catalog'
import CatalogStrategyFields from '@/components/CatalogStrategyFields'
import SecretValue from '@/components/SecretValue'

const DENSE_SOURCES = [
  { value: 'disabled', label: 'disabled — sparse only' },
  { value: 'byoe', label: 'byoe — bring your own embeddings' },
  { value: 'item2vec', label: 'item2vec — cron retrains from events' },
  { value: 'svd', label: 'svd — cron retrains via matrix factorisation' },
  { value: 'catalog', label: 'catalog — auto-embed from ingested content' },
]

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export default function CreateNamespaceDialog({ open, onOpenChange }: Props) {
  // While the freshly minted API key is on screen the dialog stops being
  // dismissible by Escape / backdrop click: the key is shown once and a stray
  // click costs the operator a key rotation to recover.
  const [keyOnScreen, setKeyOnScreen] = useState(false)

  const handleOpenChange = (next: boolean) => {
    if (!next) setKeyOnScreen(false)
    onOpenChange(next)
  }

  return (
    <Dialog
      isOpen={open}
      onOpenChange={handleOpenChange}
      width={560}
      purpose={keyOnScreen ? 'required' : 'form'}
    >
      {/*
       * Mount the body only while open so every field, the surfaced API key,
       * and the mutation state start fresh on each open — no effect-based reset
       * (which triggers cascading renders) needed.
       */}
      {open && <CreateNamespaceBody onOpenChange={handleOpenChange} onKeyShown={setKeyOnScreen} />}
    </Dialog>
  )
}

function CreateNamespaceBody({
  onOpenChange,
  onKeyShown,
}: Pick<Props, 'onOpenChange'> & { onKeyShown: (shown: boolean) => void }) {
  const navigate = useNavigate()
  const upsert = useUpsertNamespace()

  const [namespace, setNamespace] = useState('')
  const [denseSource, setDenseSource] = useState('disabled')
  const [embeddingDim, setEmbeddingDim] = useState(64)
  const [strategyId, setStrategyId] = useState('')
  const [strategyVersion, setStrategyVersion] = useState('')
  const [apiKeyShown, setApiKeyShown] = useState<string | null>(null)
  // Set when the name is already taken, or when the pre-flight lookup itself
  // failed. Both block the submit: creating is a PUT upsert, so proceeding on
  // an unknown answer risks overwriting a live namespace's config.
  const [preflightError, setPreflightError] = useState<string | null>(null)
  const [checking, setChecking] = useState(false)

  const catalogMode = denseSource === 'catalog'
  const strategies = useCatalogStrategies(embeddingDim, catalogMode)
  const descriptors = useMemo(() => strategies.data?.strategies ?? [], [strategies.data])

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setPreflightError(null)

    setChecking(true)
    try {
      const existing = await lookupNamespace(namespace)
      if (existing) {
        setPreflightError(
          `Namespace "${namespace}" already exists (dense_source=${existing.dense_source}, embedding_dim=${existing.embedding_dim}). Open it from the namespaces list to edit its config.`,
        )
        return
      }
    } catch (error) {
      setPreflightError(
        `Could not check whether "${namespace}" already exists: ${
          error instanceof Error ? error.message : 'unknown error'
        }. Not creating, since creating would overwrite an existing namespace.`,
      )
      return
    } finally {
      setChecking(false)
    }

    upsert.mutate(
      {
        namespace,
        body: {
          dense_source: denseSource,
          embedding_dim: embeddingDim,
          // The backend rejects dense_source="catalog" unless the strategy
          // rides along in the same request, so it can validate the strategy
          // dim against embedding_dim before flipping the namespace over.
          ...(catalogMode
            ? { catalog_strategy_id: strategyId, catalog_strategy_version: strategyVersion }
            : {}),
        },
      },
      {
        onSuccess: (data) => {
          if (data.api_key) {
            // First-create: the API key is returned once. Surface it here so
            // the operator can copy it before leaving the dialog.
            setApiKeyShown(data.api_key)
            onKeyShown(true)
          } else {
            onOpenChange(false)
            navigate(`/ns/${encodeURIComponent(namespace)}`)
          }
        },
      },
    )
  }

  const catalogIncomplete = catalogMode && (strategyId === '' || strategyVersion === '')
  const busy = checking || upsert.isPending

  return (
    <>
      {apiKeyShown ? (
        <Layout
          header={
            <DialogHeader
              title="Namespace created"
              subtitle="Copy the API key below — this is the only time it will be shown. Losing it means rotating the key."
            />
          }
          content={
            <LayoutContent>
              <Stack gap={6}>
                <Banner
                  status="success"
                  title={namespace}
                  description="API key (per-namespace data plane)"
                />
                <SecretValue value={apiKeyShown} label="namespace API key" />
              </Stack>
            </LayoutContent>
          }
          footer={
            <LayoutFooter>
              <Stack direction="horizontal" gap={2} align="center" hAlign="end">
                <Button variant="ghost" onClick={() => onOpenChange(false)} label="Close" />
                <Button
                  onClick={() => navigate(`/ns/${encodeURIComponent(namespace)}`)}
                  label="Open namespace"
                />
              </Stack>
            </LayoutFooter>
          }
        />
      ) : (
        <form onSubmit={onSubmit} className="contents">
          <Layout
            header={
              <DialogHeader
                title="New namespace"
                subtitle="A namespace isolates events, vectors, and trending data for one tenant."
                onOpenChange={onOpenChange}
              />
            }
            content={
              <LayoutContent>
                <Stack gap={6}>
              {preflightError && (
                <Banner status="error" title="Cannot create" description={preflightError} />
              )}
              {upsert.error && (
                <Banner
                  status="error"
                  title="Could not create namespace"
                  description={upsert.error.message}
                />
              )}

              <TextInput
                label="Namespace name"
                value={namespace}
                onChange={(next) => {
                  setNamespace(next)
                  setPreflightError(null)
                }}
                placeholder="e.g. prod"
              />

              <Selector
                label="Dense source"
                isRequired
                value={denseSource}
                onChange={(next) => {
                  setDenseSource(next)
                  setStrategyId('')
                  setStrategyVersion('')
                }}
                options={DENSE_SOURCES.map((s) => ({ value: s.value, label: s.label }))}
              />

              <NumberInput
                min={8}
                max={2048}
                value={embeddingDim}
                onChange={(next) => {
                setEmbeddingDim(next)
                // Strategies are filtered by dim, so a dim change can
                // invalidate the current pick.
                setStrategyId('')
                setStrategyVersion('')
                }}
                label="Embedding dim"
                description="Vector width for dense collections. 64 is a sane default for item2vec; 768 / 1024 typical for BYOE."
              />

              {catalogMode && (
                <CatalogStrategyFields
                  embeddingDim={embeddingDim}
                  descriptors={descriptors}
                  loading={strategies.isLoading}
                  error={strategies.error?.message}
                  strategyId={strategyId}
                  strategyVersion={strategyVersion}
                  onStrategyId={(next) => {
                    setStrategyId(next)
                    setStrategyVersion(descriptors.find((s) => s.id === next)?.version ?? '')
                  }}
                  onStrategyVersion={setStrategyVersion}
                />
              )}
                </Stack>
              </LayoutContent>
            }
            footer={
              <LayoutFooter>
                <Stack direction="horizontal" gap={2} align="center" hAlign="end">
                  <Button
                    variant="ghost"
                    type="button"
                    onClick={() => onOpenChange(false)}
                    label="Cancel"
                  />
                  <Button
                    type="submit"
                    isDisabled={busy || namespace.length === 0 || catalogIncomplete}
                    label={busy ? 'Creating…' : 'Create'}
                  />
                </Stack>
              </LayoutFooter>
            }
          />
        </form>
      )}
    </>
  )
}
