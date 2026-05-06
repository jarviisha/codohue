import type { NamespaceStatus } from '../../types'

type StatusTone = 'success' | 'warning' | 'danger' | 'neutral'

export const STATUS_META: Record<NamespaceStatus, { label: string; dot: string; tone: StatusTone }> = {
  active: { label: 'Active', dot: 'bg-success', tone: 'success' },
  idle: { label: 'Idle', dot: 'bg-muted', tone: 'neutral' },
  degraded: { label: 'Degraded', dot: 'bg-danger', tone: 'danger' },
  cold: { label: 'Cold', dot: 'bg-warning', tone: 'warning' },
}
