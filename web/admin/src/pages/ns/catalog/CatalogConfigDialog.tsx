import { useMemo, useState, type FormEvent } from 'react'
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
} from '@astryxdesign/core'
import {
  useUpdateCatalogConfig,
  type CatalogStrategyDescriptor,
  type NamespaceCatalogConfig,
  type UpdateCatalogConfigRequest,
} from '@/services/catalog'

type Props = {
  namespace: string
  open: boolean
  onOpenChange: (open: boolean) => void
  config: NamespaceCatalogConfig
  strategies: CatalogStrategyDescriptor[]
}

/**
 * CatalogConfigDialog edits a namespace's catalog auto-embedding config.
 *
 * The form mirrors internal/admin/types.go::NamespaceCatalogUpdateRequest:
 *   - strategy_id + strategy_version pickers (required)
 *   - optional max_attempts / max_content_bytes overrides
 *
 * Enabling/disabling lives on the catalog page itself, so this form always
 * submits enabled=true — it is only opened to enable-and-configure or to
 * reconfigure an already-enabled namespace.
 *
 * The form's local state is owned by the inner ConfigForm component, which
 * only mounts while the dialog is open. That way each open cycle starts
 * from the server's current values without a useEffect-driven reset.
 */
export default function CatalogConfigDialog({
  namespace,
  open,
  onOpenChange,
  config,
  strategies,
}: Props) {
  return (
    <Dialog isOpen={open} onOpenChange={onOpenChange} width={720} purpose="form">
      {open && (
        <ConfigForm
          namespace={namespace}
          config={config}
          strategies={strategies}
          onClose={() => onOpenChange(false)}
        />
      )}
    </Dialog>
  )
}

function ConfigForm({
  namespace,
  config,
  strategies,
  onClose,
}: {
  namespace: string
  config: NamespaceCatalogConfig
  strategies: CatalogStrategyDescriptor[]
  onClose: () => void
}) {
  const update = useUpdateCatalogConfig(namespace)

  const [strategyId, setStrategyId] = useState(config.strategy_id)
  const [strategyVersion, setStrategyVersion] = useState(config.strategy_version)
  const [maxAttempts, setMaxAttempts] = useState<number | null>(config.max_attempts ?? null)
  const [maxContentBytes, setMaxContentBytes] = useState<number | null>(
    config.max_content_bytes ?? null,
  )

  const strategyIds = useMemo(() => {
    const seen = new Set<string>()
    const out: string[] = []
    for (const s of strategies) {
      if (!seen.has(s.id)) {
        seen.add(s.id)
        out.push(s.id)
      }
    }
    return out
  }, [strategies])

  const versionsForStrategy = useMemo(
    () => strategies.filter((s) => s.id === strategyId),
    [strategies, strategyId],
  )

  const selectedDescriptor = useMemo(
    () => strategies.find((s) => s.id === strategyId && s.version === strategyVersion),
    [strategies, strategyId, strategyVersion],
  )

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    const body: UpdateCatalogConfigRequest = {
      enabled: true,
      strategy_id: strategyId,
      strategy_version: strategyVersion,
    }
    if (maxAttempts != null && Number.isFinite(maxAttempts) && maxAttempts > 0) {
      body.max_attempts = maxAttempts
    }
    if (maxContentBytes != null && Number.isFinite(maxContentBytes) && maxContentBytes > 0) {
      body.max_content_bytes = maxContentBytes
    }
    update.mutate(body, {
      onSuccess: () => onClose(),
    })
  }

  const canSubmit = strategyId !== '' && strategyVersion !== ''

  return (
    <form onSubmit={onSubmit} className="contents">
      <Layout
        header={
          <DialogHeader
            title={`Catalog auto-embedding for ${namespace}`}
            subtitle="Saving routes ingested content through the embedder worker with the selected strategy. Use the Disable button on the catalog page to turn auto-embedding off."
            onOpenChange={onClose}
          />
        }
        content={
          <LayoutContent>
            <Stack gap={6}>
              {update.error && (
                <Banner
                  status="error"
                  title="Update failed"
                  description={update.error.message}
                />
              )}

              <Selector
                label="Strategy"
                isRequired
                description="Identifies the embed strategy (model family). Choose the version below."
                placeholder="— select strategy —"
                value={strategyId}
                onChange={(next) => {
                  setStrategyId(next)
                  const firstVersion = strategies.find((s) => s.id === next)?.version ?? ''
                  setStrategyVersion(firstVersion)
                }}
                options={strategyIds}
              />

              <Selector
                label="Strategy version"
                isRequired
                description={
                  selectedDescriptor
                    ? `dim ${selectedDescriptor.dim}${selectedDescriptor.description ? ` — ${selectedDescriptor.description}` : ''}`
                    : 'Pick a strategy first.'
                }
                placeholder="— select version —"
                value={strategyVersion}
                onChange={setStrategyVersion}
                isDisabled={strategyId === ''}
                options={versionsForStrategy.map((s) => ({
                  value: s.version,
                  label: `${s.version}${s.default ? ' (default)' : ''}`,
                }))}
              />

              <NumberInput
                min={1}
                max={20}
                value={maxAttempts}
                onChange={(next) => setMaxAttempts(next)}
                label="Max attempts"
                description="Transient retries before the item moves to dead-letter. Leave blank to inherit the server default."
                placeholder="default"
              />

              <NumberInput
                min={1024}
                value={maxContentBytes}
                onChange={(next) => setMaxContentBytes(next)}
                label="Max content bytes"
                description="Per-item content cap enforced at ingest. Leave blank to inherit CODOHUE_CATALOG_MAX_CONTENT_BYTES."
                placeholder="default"
              />
            </Stack>
          </LayoutContent>
        }
        footer={
          <LayoutFooter>
            <Stack direction="horizontal" gap={2} align="center" hAlign="end">
              <Button type="button" variant="ghost" onClick={onClose} label="Cancel" />
              <Button
                type="submit"
                isDisabled={!canSubmit || update.isPending}
                label={update.isPending ? 'Saving…' : 'Save'}
              />
            </Stack>
          </LayoutFooter>
        }
      />
    </form>
  )
}
