type Values = Record<string, unknown>
export type Draft = Record<string, string | boolean | null>
export function toDraft(values: Values): Draft {
  return Object.fromEntries(
    Object.entries(values).map(([k, v]) => [
      k,
      v === null
        ? null
        : typeof v === 'boolean'
          ? v
          : typeof v === 'object'
            ? JSON.stringify(v, null, 2)
            : String(v),
    ]),
  )
}
export function draftChanges(baseline: Draft, draft: Draft): string[] {
  return Object.keys(draft).filter((k) => draft[k] !== baseline[k])
}
export function parseChanges(
  baseline: Draft,
  draft: Draft,
): { changes: Values; errors: Record<string, string> } {
  const changes: Values = {},
    errors: Record<string, string> = {}
  for (const key of draftChanges(baseline, draft)) {
    const value = draft[key]
    if (value === null || typeof value === 'boolean') {
      changes[key] = value
      continue
    }
    if (
      [
        'dense_source',
        'dense_distance',
        'catalog_strategy_id',
        'catalog_strategy_version',
      ].includes(key)
    ) {
      changes[key] = value
      continue
    }
    if (['action_weights', 'catalog_strategy_params'].includes(key)) {
      try {
        const parsed: unknown = JSON.parse(value)
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error()
        changes[key] = parsed
      } catch {
        errors[key] =
          key === 'action_weights'
            ? 'Each action needs a unique name and a finite numeric weight.'
            : 'Enter a valid JSON object.'
      }
      continue
    }
    const number = Number(value)
    if (!value.trim() || !Number.isFinite(number)) errors[key] = 'Enter a finite number.'
    else if (
      !['alpha', 'gamma', 'lambda', 'lambda_trending'].includes(key) &&
      !Number.isSafeInteger(number)
    )
      errors[key] = 'Enter a whole number.'
    else changes[key] = number
  }
  return { changes, errors }
}
