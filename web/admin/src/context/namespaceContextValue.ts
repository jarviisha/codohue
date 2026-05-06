import { createContext } from 'react'

export interface NamespaceContextValue {
  namespace: string
  setNamespace: (ns: string) => void
}

export const NamespaceContext = createContext<NamespaceContextValue | null>(null)
