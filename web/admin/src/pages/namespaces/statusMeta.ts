import type { NamespaceStatus } from '../../types'

export const STATUS_META: Record<NamespaceStatus, { label: string; wrap: string; dot: string; text: string }> = {
  active: { label: 'Active', wrap: 'bg-success-bg border border-success/30', dot: 'bg-success', text: 'text-success' },
  idle: { label: 'Idle', wrap: 'bg-accent-subtle border border-accent/20', dot: 'bg-accent', text: 'text-accent' },
  degraded: { label: 'Degraded', wrap: 'bg-danger-bg border border-danger/25', dot: 'bg-danger', text: 'text-danger' },
  cold: { label: 'Cold', wrap: 'bg-warning-bg border border-warning/30', dot: 'bg-warning', text: 'text-warning' },
}
