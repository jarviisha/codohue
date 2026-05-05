import { createContext, useContext, useState, type ReactNode } from 'react'

const STORAGE_KEY = 'codohue_active_ns'

interface NamespaceContextValue {
  namespace: string
  setNamespace: (ns: string) => void
}

const NamespaceContext = createContext<NamespaceContextValue | null>(null)

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

export function useActiveNamespace() {
  const ctx = useContext(NamespaceContext)
  if (!ctx) throw new Error('useActiveNamespace must be used inside NamespaceProvider')
  return ctx
}
