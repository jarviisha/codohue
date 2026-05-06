export function formatCount(value: number | null | undefined): string {
  if (value == null) return '—'
  return value.toLocaleString()
}

export function formatDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

export function formatDateTimeWithZone(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
  const offset = -d.getTimezoneOffset()
  const tzLabel = offset === 0 ? 'UTC' : `UTC${offset > 0 ? '+' : ''}${offset / 60}`
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())} (${tz || tzLabel})`
}

export function formatDateTimeShort(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function formatTimeOfDay(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString()
}

export function formatDurationMs(value: number | null | undefined): string {
  return value != null ? `${value} ms` : '—'
}

export function formatRelativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const mins = Math.floor(diff / 60_000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

export function formatTTL(ttlSec: number): string {
  if (ttlSec === -2) return 'no cache'
  if (ttlSec === -1) return 'no expiry'
  const m = Math.floor(ttlSec / 60)
  const s = ttlSec % 60
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}
