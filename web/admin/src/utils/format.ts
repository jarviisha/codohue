// Shared display formatters. Pure functions — UI files import what they
// need rather than redefining them.
//
// Conventions:
// - "—" (em dash) is the universal "no value" placeholder for missing data.
// - Locale defaults to en-US so operators worldwide see the same thousands
//   separators (and our mono tabular-nums alignment stays stable).
// - All time helpers accept ISO-8601 strings; falsy input returns "—".

const PLACEHOLDER = '—'
const LOCALE = 'en-US'

/**
 * Human-readable "Xs ago" / "Xm ago" / "Xh ago" / "Xd ago" relative time.
 * Returns "just now" for any timestamp at or in the very-near future.
 * Returns "—" for falsy or unparseable input.
 */
export function formatRelative(iso: string | null | undefined): string {
  if (!iso) return PLACEHOLDER
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return PLACEHOLDER
  const delta = Date.now() - t
  if (delta < 0) return 'just now'
  const s = Math.floor(delta / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  return `${d}d ago`
}

/**
 * Human-readable duration from a count of milliseconds. Sub-second values
 * stay in ms ("812ms"); anything 1s or longer becomes seconds with three
 * decimal places ("4.812s"). Returns "—" for null / undefined.
 */
export function formatDurationMs(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return PLACEHOLDER
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(3)}s`
}

/**
 * Engineer-facing absolute timestamp. ISO-8601 with the `T` replaced by a
 * space and `Z` replaced by " UTC" so it reads as
 * `2026-05-13 14:02:38.412 UTC`. Falls back to the raw input if unparseable,
 * "—" if falsy.
 */
export function formatTimestamp(iso: string | null | undefined): string {
  if (!iso) return PLACEHOLDER
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toISOString().replace('T', ' ').replace('Z', ' UTC')
}

/**
 * Locale-formatted integer or float ("12,418"). Returns "—" for null /
 * undefined / NaN.
 */
export function formatNumber(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return PLACEHOLDER
  return n.toLocaleString(LOCALE)
}
