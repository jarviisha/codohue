import { useState, type ReactNode } from 'react'
import { NamespaceContext } from './namespaceContextValue'

const STORAGE_KEY = 'codohue_active_ns'

export function NamespaceProvider({ children }: { children: ReactNode }) {
  const [namespace, setNamespaceState] = useState(() => localStorage.getItem(STORAGE_KEY) ?? '')

  function setNamespace(ns: string) {
    localStorage.setItem(STORAGE_KEY, ns)
    setNamespaceState(ns)
  }

  return (
    <NamespaceContext.Provider value={{ namespace, setNamespace }}>
      {children}
    </NamespaceContext.Provider>
  )
}
