import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  Button,
  ConfirmDialog,
  Field,
  Form,
  FormGrid,
  KeyValueList,
  LoadingState,
  Notice,
  NumberInput,
  PageHeader,
  PageShell,
  Panel,
  Select,
  StatusToken,
  Switch,
  useRegisterCommand,
} from '../../../components/ui'
import type {
  CatalogStrategyDescriptor,
  NamespaceCatalogResponse,
} from '../../../services/catalog'
import {
  useCatalogConfig,
  useTriggerCatalogReEmbed,
  useUpdateCatalogConfig,
} from '../../../services/catalog'
import { ApiError } from '../../../services/http'
import { paths } from '../../../routes/path'
import { formatNumber, formatRelative } from '../../../utils/format'

const FORM_ID = 'catalog-config-form'

interface FormState {
  enabled: boolean
  strategyKey: string
  paramsText: string
  maxAttempts: string
  maxContentBytes: string
}

interface DimMismatchBody {
  error?: string
  strategy_dim?: number
  namespace_embedding_dim?: number
}

function strategyKey(strategy: CatalogStrategyDescriptor) {
  return `${strategy.id}@${strategy.version}`
}

function splitStrategyKey(key: string) {
  const idx = key.lastIndexOf('@')
  if (idx < 0) return { id: key, version: '' }
  return { id: key.slice(0, idx), version: key.slice(idx + 1) }
}

function formatParams(params: Record<string, unknown> | undefined) {
  if (!params || Object.keys(params).length === 0) return '{}'
  return JSON.stringify(params, null, 2)
}

function initialForm(data: NamespaceCatalogResponse): FormState {
  const currentKey =
    data.catalog.strategy_id && data.catalog.strategy_version
      ? `${data.catalog.strategy_id}@${data.catalog.strategy_version}`
      : strategyKey(data.available_strategies[0] ?? { id: '', version: '', dim: 0 })

  return {
    enabled: data.catalog.enabled,
    strategyKey: currentKey,
    paramsText: formatParams(data.catalog.params),
    maxAttempts: String(data.catalog.max_attempts),
    maxContentBytes: String(data.catalog.max_content_bytes),
  }
}

function dimMismatch(error: unknown): DimMismatchBody | null {
  if (!(error instanceof ApiError) || error.status !== 400) return null
  const body = error.body as DimMismatchBody | null
  if (
    typeof body?.strategy_dim === 'number' &&
    typeof body?.namespace_embedding_dim === 'number'
  ) {
    return body
  }
  return null
}

function backlogState(count: number) {
  return count > 0 ? 'warn' : 'ok'
}

export default function CatalogConfigPage() {
  const { name = '' } = useParams<{ name: string }>()
  const catalog = useCatalogConfig(name)

  if (catalog.isLoading) {
    return (
      <PageShell>
        <PageHeader title="Catalog" meta={`namespace ${name}`} />
        <LoadingState rows={7} label="loading catalog config" />
      </PageShell>
    )
  }

  if (catalog.isError || !catalog.data) {
    return (
      <PageShell>
        <PageHeader title="Catalog" meta={`namespace ${name}`} />
        <Notice tone="fail" title="Failed to load catalog config">
          {(catalog.error as Error)?.message ?? 'Unable to load catalog state.'}
        </Notice>
      </PageShell>
    )
  }

  return (
    <CatalogConfigEditor
      data={catalog.data}
      isFetching={catalog.isFetching}
      refetch={() => void catalog.refetch()}
    />
  )
}

function CatalogConfigEditor({
  data,
  isFetching,
  refetch,
}: {
  data: NamespaceCatalogResponse
  isFetching: boolean
  refetch: () => void
}) {
  const { name = '' } = useParams<{ name: string }>()
  const navigate = useNavigate()
  const updateCatalog = useUpdateCatalogConfig()
  const reEmbed = useTriggerCatalogReEmbed()

  const [form, setForm] = useState<FormState>(() => initialForm(data))
  const [paramsError, setParamsError] = useState<string | null>(null)
  const [showReEmbedConfirm, setShowReEmbedConfirm] = useState(false)
  const [saved, setSaved] = useState(false)

  useRegisterCommand(
    `ns.${name}.catalog.items`,
    `Open ${name} catalog items`,
    () => navigate(paths.nsCatalogItems(name)),
    name,
  )
  useRegisterCommand(
    `ns.${name}.catalog.refresh`,
    `Refresh ${name} catalog`,
    refetch,
    name,
  )
  useRegisterCommand(
    `ns.${name}.catalog.reembed`,
    `Re-embed ${name} catalog`,
    () => setShowReEmbedConfirm(true),
    name,
  )

  const selectedStrategy = data.available_strategies.find(
    (strategy) => strategyKey(strategy) === form.strategyKey,
  )

  const save = () => {
    setSaved(false)
    setParamsError(null)

    let params: Record<string, unknown> | undefined
    try {
      const parsed = JSON.parse(form.paramsText || '{}') as unknown
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
        setParamsError('params must be a JSON object')
        return
      }
      params = parsed as Record<string, unknown>
    } catch {
      setParamsError('params must be valid JSON')
      return
    }

    const picked = splitStrategyKey(form.strategyKey)
    updateCatalog.mutate(
      {
        namespace: name,
        payload: {
          enabled: form.enabled,
          strategy_id: picked.id || null,
          strategy_version: picked.version || null,
          params,
          max_attempts: Number(form.maxAttempts),
          max_content_bytes: Number(form.maxContentBytes),
        },
      },
      {
        onSuccess: () => {
          setSaved(true)
        },
      },
    )
  }

  const mismatch = dimMismatch(updateCatalog.error)
  const updateError =
    mismatch === null && updateCatalog.error instanceof Error
      ? updateCatalog.error.message
      : null
  const reEmbedError =
    reEmbed.error instanceof ApiError && reEmbed.error.status === 409
      ? 'A re-embed is already running for this namespace.'
      : reEmbed.error instanceof Error
        ? reEmbed.error.message
        : null

  return (
    <PageShell>
      <PageHeader
        title="Catalog"
        meta={`namespace ${name}`}
        actions={
          <>
            <Button
              variant="ghost"
              size="sm"
              loading={isFetching}
              onClick={refetch}
            >
              Refresh
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => navigate(paths.nsCatalogItems(name))}
            >
              Items
            </Button>
            <Button
              variant="primary"
              size="sm"
              disabled={!data.catalog.enabled}
              onClick={() => setShowReEmbedConfirm(true)}
            >
              Re-embed all
            </Button>
          </>
        }
      />

      {saved ? (
        <Notice
          tone="ok"
          title="Catalog config saved"
          onDismiss={() => setSaved(false)}
        >
          Updated catalog auto-embedding settings for {name}.
        </Notice>
      ) : null}

      {mismatch ? (
        <Notice tone="fail" title="Strategy dimension mismatch">
          Strategy emits {mismatch.strategy_dim} dimensions, but namespace {name}{' '}
          expects {mismatch.namespace_embedding_dim}.
        </Notice>
      ) : updateError ? (
        <Notice tone="fail" title="Save failed">
          {updateError}
        </Notice>
      ) : null}

      {reEmbed.isSuccess && reEmbed.data ? (
        <Notice
          tone="ok"
          title={`Re-embed batch #${reEmbed.data.batch_run_id} queued`}
          onDismiss={() => reEmbed.reset()}
        >
          {formatNumber(reEmbed.data.stale_items)} catalog items were reset for
          embedding with {reEmbed.data.strategy_id}@{reEmbed.data.strategy_version}.
        </Notice>
      ) : reEmbedError ? (
        <Notice tone="warn" title="Re-embed not started">
          {reEmbedError}
        </Notice>
      ) : null}

      <div className="grid grid-cols-1 xl:grid-cols-[minmax(0,1fr)_360px] gap-4">
        <Panel title="auto-embedding config">
          <Form
            id={FORM_ID}
            onSubmit={(event) => {
              event.preventDefault()
              save()
            }}
          >
            <div className="flex items-center justify-between gap-4 rounded-sm border border-default bg-surface-raised px-3 py-2">
              <div>
                <p className="text-sm font-semibold text-primary">
                  Catalog auto-embedding
                </p>
                <p className="text-sm text-muted">
                  Enable the embedder worker path for raw catalog content.
                </p>
              </div>
              <Switch
                checked={form.enabled}
                ariaLabel="Toggle catalog auto-embedding"
                onChange={(enabled) =>
                  setForm((prev) => ({ ...prev, enabled }))
                }
              />
            </div>

            <Field
              label="strategy"
              htmlFor="catalog-strategy"
              hint={
                selectedStrategy
                  ? `${selectedStrategy.dim} dim · max input ${formatNumber(selectedStrategy.max_input_bytes)} bytes`
                  : 'No compatible strategies are currently registered.'
              }
              required={form.enabled}
            >
              <Select
                id="catalog-strategy"
                value={form.strategyKey}
                disabled={data.available_strategies.length === 0}
                onChange={(event) =>
                  setForm((prev) => ({
                    ...prev,
                    strategyKey: event.target.value,
                  }))
                }
              >
                {data.available_strategies.map((strategy) => (
                  <option key={strategyKey(strategy)} value={strategyKey(strategy)}>
                    {strategy.id}@{strategy.version}
                  </option>
                ))}
              </Select>
            </Field>

            <FormGrid>
              <Field label="max attempts" htmlFor="catalog-max-attempts">
                <NumberInput
                  id="catalog-max-attempts"
                  min={1}
                  max={100}
                  step={1}
                  value={form.maxAttempts}
                  onChange={(event) =>
                    setForm((prev) => ({
                      ...prev,
                      maxAttempts: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field label="max content bytes" htmlFor="catalog-max-content-bytes">
                <NumberInput
                  id="catalog-max-content-bytes"
                  min={1}
                  step={1024}
                  width="w-36"
                  value={form.maxContentBytes}
                  onChange={(event) =>
                    setForm((prev) => ({
                      ...prev,
                      maxContentBytes: event.target.value,
                    }))
                  }
                />
              </Field>
            </FormGrid>

            <Field
              label="strategy params"
              htmlFor="catalog-params"
              error={paramsError}
              hint="JSON object passed to the selected embedding strategy."
            >
              <textarea
                id="catalog-params"
                className={[
                  'min-h-32 rounded-sm border bg-surface p-3 font-mono text-sm text-primary',
                  'focus:outline-none focus:shadow-focus',
                  paramsError
                    ? 'border-danger'
                    : 'border-default hover:border-strong focus:border-accent',
                ].join(' ')}
                spellCheck={false}
                value={form.paramsText}
                onChange={(event) =>
                  setForm((prev) => ({
                    ...prev,
                    paramsText: event.target.value,
                  }))
                }
              />
            </Field>

            <div className="flex justify-end">
              <Button
                type="submit"
                variant="primary"
                loading={updateCatalog.isPending}
              >
                Save catalog config
              </Button>
            </div>
          </Form>
        </Panel>

        <div className="flex flex-col gap-4">
          <Panel title="status">
            <KeyValueList
              rows={[
                {
                  label: 'enabled',
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <StatusToken state={data.catalog.enabled ? 'ok' : 'idle'} />
                      <span>{data.catalog.enabled ? 'enabled' : 'disabled'}</span>
                    </span>
                  ),
                },
                {
                  label: 'strategy',
                  value:
                    data.catalog.strategy_id && data.catalog.strategy_version
                      ? `${data.catalog.strategy_id}@${data.catalog.strategy_version}`
                      : 'none',
                },
                {
                  label: 'embedding_dim',
                  value: data.catalog.embedding_dim.toString(),
                },
                { label: 'updated', value: formatRelative(data.catalog.updated_at) },
              ]}
            />
          </Panel>

          <Panel title="backlog">
            <KeyValueList
              rows={[
                {
                  label: 'pending',
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <StatusToken state={backlogState(data.backlog.pending)} />
                      <span>{formatNumber(data.backlog.pending)}</span>
                    </span>
                  ),
                },
                { label: 'in_flight', value: formatNumber(data.backlog.in_flight) },
                { label: 'embedded', value: formatNumber(data.backlog.embedded) },
                {
                  label: 'failed',
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <StatusToken state={data.backlog.failed > 0 ? 'fail' : 'ok'} />
                      <span>{formatNumber(data.backlog.failed)}</span>
                    </span>
                  ),
                },
                {
                  label: 'dead_letter',
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <StatusToken
                        state={data.backlog.dead_letter > 0 ? 'fail' : 'ok'}
                      />
                      <span>{formatNumber(data.backlog.dead_letter)}</span>
                    </span>
                  ),
                },
                { label: 'stream_len', value: formatNumber(data.backlog.stream_len) },
              ]}
            />
          </Panel>
        </div>
      </div>

      <ConfirmDialog
        open={showReEmbedConfirm}
        title="Re-embed all catalog items?"
        description="Every stale catalog item will be reset for the active strategy and queued through the embedder stream."
        confirmLabel="Re-embed all"
        loading={reEmbed.isPending}
        onConfirm={() =>
          reEmbed.mutate(name, {
            onSettled: () => setShowReEmbedConfirm(false),
          })
        }
        onCancel={() => setShowReEmbedConfirm(false)}
      />
    </PageShell>
  )
}
