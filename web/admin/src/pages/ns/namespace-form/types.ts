import type {
  NamespaceFormErrors,
  NamespaceFormState,
} from '../configForm'

export type TabId = 'identity' | 'actions' | 'scoring' | 'dense' | 'trending'

// Shared props every tab receives from the orchestrator. Tabs compose their
// own field-level setters out of `update` and `updateNumber` so each tab file
// stays focused on its own field layout.
export interface TabProps {
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
