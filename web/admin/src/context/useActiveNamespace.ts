import { useContext } from 'react'
import { NamespaceContext } from './namespaceContextValue'

export function useActiveNamespace() {
  const ctx = useContext(NamespaceContext)
  if (!ctx) throw new Error('useActiveNamespace must be used inside NamespaceProvider')
  return ctx
}
