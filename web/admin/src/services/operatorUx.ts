/** Keep section navigation independent of namespace-specific entity IDs. */
export function safeNamespaceSection(pathname: string): string {
  const parts = pathname.split('/').filter(Boolean)
  if (parts[0] !== 'ns' || !parts[1]) return ''
  const section = parts[2]
  if (section === 'catalog') return parts[3] === 'items' ? '/catalog/items' : '/catalog'
  return ['subjects', 'batch-runs', 'events', 'trending', 'config'].includes(section)
    ? `/${section}`
    : ''
}

export type ActionWeightRow = { id: string; name: string; value: number }
export type FieldError = { id: string; message: string }

export function validateActionWeights(rows: ActionWeightRow[]): FieldError[] {
  const errors: FieldError[] = []
  const counts = new Map<string, number>()
  for (const row of rows) {
    const name = row.name.trim()
    counts.set(name, (counts.get(name) ?? 0) + 1)
  }
  for (const row of rows) {
    const name = row.name.trim()
    if (!name || counts.get(name)! > 1) {
      errors.push({
        id: `action-${row.id}`,
        message: !name ? 'Enter an action name.' : `Action "${name}" is duplicated.`,
      })
    }
    if (!Number.isFinite(row.value)) {
      errors.push({
        id: `weight-${row.id}`,
        message: `Enter a finite weight for ${name || 'this action'}.`,
      })
    }
  }
  return errors
}

export function readPage(value: string | null): number {
  if (!value || !/^\d+$/.test(value)) return 0
  const page = Number(value)
  return Number.isSafeInteger(page) && page > 0 && page <= 1_000_000 ? page - 1 : 0
}
