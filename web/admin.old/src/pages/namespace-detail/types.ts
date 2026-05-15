import type { NamespaceFormState } from '../namespaceForm'

export type UpdateNamespaceField = <K extends keyof NamespaceFormState>(
  field: K,
  value: NamespaceFormState[K],
) => void

export type UpdateNamespaceNumber = (field: keyof NamespaceFormState, value: string) => void
