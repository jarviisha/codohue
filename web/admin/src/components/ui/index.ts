// Barrel for shared UI primitives. Page files compose from this single
// import path — see DESIGN.md §6 for the catalogue.

// Layout
export { default as PageShell } from './PageShell'
export { default as PageHeader } from './PageHeader'

// Content surfaces
export { default as Panel } from './Panel'
export { default as Toolbar } from './Toolbar'
export { default as MetricTile } from './MetricTile'
export { default as Badge } from './Badge'
export { default as KeyValueList } from './KeyValueList'
export type { KeyValueRow } from './KeyValueList'
export { default as CodeBadge } from './CodeBadge'
export { default as EmptyState } from './EmptyState'
export { default as LoadingState } from './LoadingState'
export { default as Notice } from './Notice'

// Table family
export { Table, Thead, Tbody, Tr, Th, Td } from './Table'

// Forms
export { default as Field } from './Field'
export { default as Form } from './Form'
export { default as FormGrid } from './FormGrid'
export { default as Input } from './Input'
export { default as Select } from './Select'
export { default as NumberInput } from './NumberInput'
export { default as Checkbox } from './Checkbox'
export { default as Radio, RadioGroup } from './Radio'
export type { RadioOption } from './Radio'
export { default as Switch } from './Switch'

// Overlays
export { default as Button } from './Button'
export { default as Modal } from './Modal'
export { default as ConfirmDialog } from './ConfirmDialog'
export { default as Dropdown, DropdownItem } from './Dropdown'
export { default as CommandPalette, useRegisterCommand } from './CommandPalette'
export type { Command } from './CommandPalette'

// Status + signals
export { default as StatusToken } from './StatusToken'
export type { StatusState } from './StatusToken'
export { default as Kbd } from './Kbd'

// Navigation aids
export { default as Pagination } from './Pagination'

// Data display
export { default as CodeBlock } from './CodeBlock'
