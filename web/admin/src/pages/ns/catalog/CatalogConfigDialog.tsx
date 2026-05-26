import { useMemo, useState, type FormEvent } from 'react'
import {
  Alert,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  FormField,
  Inline,
  Input,
  Select,
  Stack,
  Switch,
} from '@jarviisha/davinci-react-ui'
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
 *   - enabled toggle
 *   - strategy_id + strategy_version pickers (required when enabling)
 *   - optional max_attempts / max_content_bytes overrides
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
    <Dialog open={open} onOpenChange={onOpenChange} size="md">
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

  const [enabled, setEnabled] = useState(config.enabled)
  const [strategyId, setStrategyId] = useState(config.strategy_id)
  const [strategyVersion, setStrategyVersion] = useState(config.strategy_version)
  const [maxAttempts, setMaxAttempts] = useState<string>(
    config.max_attempts != null ? String(config.max_attempts) : '',
  )
  const [maxContentBytes, setMaxContentBytes] = useState<string>(
    config.max_content_bytes != null ? String(config.max_content_bytes) : '',
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
    const body: UpdateCatalogConfigRequest = { enabled }
    if (enabled) {
      body.strategy_id = strategyId
      body.strategy_version = strategyVersion
    }
    if (maxAttempts !== '') {
      const n = Number(maxAttempts)
      if (Number.isFinite(n) && n > 0) body.max_attempts = n
    }
    if (maxContentBytes !== '') {
      const n = Number(maxContentBytes)
      if (Number.isFinite(n) && n > 0) body.max_content_bytes = n
    }
    update.mutate(body, {
      onSuccess: () => onClose(),
    })
  }

  const canSubmit = !enabled || (strategyId !== '' && strategyVersion !== '')

  return (
    <form onSubmit={onSubmit}>
      <DialogHeader>
        <DialogTitle>Catalog auto-embedding · {namespace}</DialogTitle>
        <DialogDescription>
          Enabling catalog routes ingested content through the embedder worker. Disabling stops new
          embedding but keeps existing vectors in Qdrant.
        </DialogDescription>
      </DialogHeader>
      <DialogContent>
        <Stack gap="200">
          {update.error && (
            <Alert
              variant="danger"
              title="Update failed"
              description={update.error.message}
            />
          )}

          <FormField
            label="Catalog enabled"
            helpText="When off, POST /v1/namespaces/{ns}/catalog returns 503 and no auto-embedding runs."
          >
            <Switch
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              label={enabled ? 'On' : 'Off'}
            />
          </FormField>

          <FormField
            label="Strategy"
            required={enabled}
            helpText="Identifies the embed strategy (model family). Choose the version below."
          >
            <Select
              value={strategyId}
              onChange={(e) => {
                const next = e.target.value
                setStrategyId(next)
                const firstVersion = strategies.find((s) => s.id === next)?.version ?? ''
                setStrategyVersion(firstVersion)
              }}
              disabled={!enabled}
            >
              <option value="">— select strategy —</option>
              {strategyIds.map((id) => (
                <option key={id} value={id}>
                  {id}
                </option>
              ))}
            </Select>
          </FormField>

          <FormField
            label="Strategy version"
            required={enabled}
            helpText={
              selectedDescriptor
                ? `dim ${selectedDescriptor.dim}${selectedDescriptor.description ? ' · ' + selectedDescriptor.description : ''}`
                : 'Pick a strategy first.'
            }
          >
            <Select
              value={strategyVersion}
              onChange={(e) => setStrategyVersion(e.target.value)}
              disabled={!enabled || strategyId === ''}
            >
              <option value="">— select version —</option>
              {versionsForStrategy.map((s) => (
                <option key={s.version} value={s.version}>
                  {s.version}
                  {s.default ? ' (default)' : ''}
                </option>
              ))}
            </Select>
          </FormField>

          <FormField
            label="Max attempts"
            helpText="Transient retries before the item moves to dead-letter. Leave blank to inherit the server default."
          >
            <Input
              type="number"
              min={1}
              max={20}
              value={maxAttempts}
              onChange={(e) => setMaxAttempts(e.target.value)}
              placeholder="default"
            />
          </FormField>

          <FormField
            label="Max content bytes"
            helpText="Per-item content cap enforced at ingest. Leave blank to inherit CODOHUE_CATALOG_MAX_CONTENT_BYTES."
          >
            <Input
              type="number"
              min={1024}
              value={maxContentBytes}
              onChange={(e) => setMaxContentBytes(e.target.value)}
              placeholder="default"
            />
          </FormField>
        </Stack>
      </DialogContent>
      <DialogFooter>
        <Inline gap="100" justify="end">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!canSubmit || update.isPending}>
            {update.isPending ? 'Saving…' : 'Save'}
          </Button>
        </Inline>
      </DialogFooter>
    </form>
  )
}
