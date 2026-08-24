import { Alert, FormField, Select } from '@jarviisha/davinci-react-ui'
import type { CatalogStrategyDescriptor } from '@/services/catalog'

/**
 * CatalogStrategyFields renders the strategy + version pickers that
 * dense_source="catalog" requires.
 *
 * It exists because the backend refuses dense_source="catalog" unless
 * catalog_strategy_id + catalog_strategy_version accompany it in the same
 * request — it validates the strategy's dim against the namespace's
 * embedding_dim before flipping the namespace into catalog mode. Offering
 * "catalog" in a dense-source picker without these fields produces a form that
 * can only ever fail, so the two travel together.
 *
 * Descriptors come from the namespace-free registry endpoint, which is what
 * lets the same component serve the create form (no namespace yet) and the
 * config page.
 */
export default function CatalogStrategyFields({
  embeddingDim,
  descriptors,
  loading,
  error,
  strategyId,
  strategyVersion,
  onStrategyId,
  onStrategyVersion,
}: {
  embeddingDim: number
  descriptors: CatalogStrategyDescriptor[]
  loading: boolean
  error: string | undefined
  strategyId: string
  strategyVersion: string
  onStrategyId: (next: string) => void
  onStrategyVersion: (next: string) => void
}) {
  if (error) {
    return (
      <Alert
        variant="danger"
        title="Could not load embed strategies"
        description={`${error}. Catalog mode needs a strategy, so pick another dense source or retry.`}
      />
    )
  }
  if (!loading && descriptors.length === 0) {
    return (
      <Alert
        variant="warning"
        title="No strategy matches this embedding dim"
        description={`No registered embed strategy produces ${embeddingDim}-dimensional vectors. Change the embedding dim to match a registered strategy, or choose another dense source.`}
      />
    )
  }

  const ids: string[] = []
  for (const d of descriptors) {
    if (!ids.includes(d.id)) ids.push(d.id)
  }
  const versions = descriptors.filter((d) => d.id === strategyId)
  const selected = versions.find((v) => v.version === strategyVersion)

  return (
    <>
      <FormField
        label="Catalog strategy"
        required
        helpText="Which embed strategy turns ingested content into vectors. Only strategies matching the embedding dim above are listed."
      >
        <Select value={strategyId} onChange={(e) => onStrategyId(e.target.value)} disabled={loading}>
          <option value="">{loading ? 'loading…' : '— select strategy —'}</option>
          {ids.map((id) => (
            <option key={id} value={id}>
              {id}
            </option>
          ))}
        </Select>
      </FormField>

      <FormField
        label="Strategy version"
        required
        helpText={
          selected
            ? `dim ${selected.dim}${selected.description ? ` — ${selected.description}` : ''}`
            : 'Pick a strategy first.'
        }
      >
        <Select
          value={strategyVersion}
          onChange={(e) => onStrategyVersion(e.target.value)}
          disabled={strategyId === ''}
        >
          <option value="">— select version —</option>
          {versions.map((v) => (
            <option key={v.version} value={v.version}>
              {v.version}
            </option>
          ))}
        </Select>
      </FormField>
    </>
  )
}
