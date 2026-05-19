import type {
  NamespaceFormErrors,
  NamespaceFormState,
} from '../configForm'

// Shared props every section receives from the orchestrator. Sections compose
// their own field-level setters out of `update` and `updateNumber` so each
// file stays focused on its own field layout.
export interface SectionProps {
  state: NamespaceFormState
  errors: NamespaceFormErrors
  update: <K extends keyof NamespaceFormState>(
    key: K,
    value: NamespaceFormState[K],
  ) => void
  updateNumber: <K extends keyof NamespaceFormState>(
    key: K,
    raw: string,
  ) => void
}
