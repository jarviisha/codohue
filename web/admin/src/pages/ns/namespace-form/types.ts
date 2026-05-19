import type {
  NamespaceFormErrors,
  NamespaceFormState,
} from '../configForm'

// Read-only context about constraints that live outside the form state but
// still need to gate which fields the operator can edit. Today this only
// carries catalog auto-embedding state (when catalog is enabled the backend
// rejects item2vec/svd and pins embedding_dim to the strategy dim).
export interface FormContext {
  catalogEnabled: boolean
  catalogStrategyId?: string
  catalogStrategyVersion?: string
}

// Shared props every section receives from the orchestrator. Sections compose
// their own field-level setters out of `update` and `updateNumber` so each
// file stays focused on its own field layout.
export interface SectionProps {
  state: NamespaceFormState
  errors: NamespaceFormErrors
  context: FormContext
  update: <K extends keyof NamespaceFormState>(
    key: K,
    value: NamespaceFormState[K],
  ) => void
  updateNumber: <K extends keyof NamespaceFormState>(
    key: K,
    raw: string,
  ) => void
}
