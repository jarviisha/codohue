import { useMemo } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { Selector } from '@astryxdesign/core'
import { useNamespaces } from '@/services/namespaces'
import { useRecentNamespaces } from '@/services/recentNamespaces'
import useNamespaceParam from '@/components/shell/useNamespaceParam'

/**
 * NamespaceSwitcher is the shell's context switcher: namespace is the tenant
 * dimension every namespace-scoped page hangs off, so it lives in the TopNav
 * where it stays visible on every route — including fleet-level ones, where it
 * reads as "pick a namespace to drill into".
 *
 * It used to sit inside the sidebar and only appear once you were already in a
 * namespace, which meant the only way in was the /namespaces list.
 */
export default function NamespaceSwitcher() {
  const currentNs = useNamespaceParam()
  const location = useLocation()
  const navigate = useNavigate()
  const nsList = useNamespaces()
  const recents = useRecentNamespaces()

  // Sub-path retained on namespace switch: everything after /ns/{currentNs},
  // truncated at the first id-shaped (numeric) segment so cross-namespace
  // jumps don't carry ids unique to the source ns.
  const subPath = useMemo(() => {
    if (!currentNs) return ''
    const segs = location.pathname.split('/').filter(Boolean)
    if (segs[0] !== 'ns' || segs[1] !== currentNs) return ''
    const after = segs.slice(2)
    const firstNumIdx = after.findIndex((s) => /^\d+$/.test(s))
    const safe = firstNumIdx >= 0 ? after.slice(0, firstNumIdx) : after
    return safe.length > 0 ? '/' + safe.join('/') : ''
  }, [location.pathname, currentNs])

  // Recent-first (excluding the current one), then the rest alphabetically.
  // Operators bouncing between two namespaces get a one-keystroke switch; the
  // long tail stays reachable through the search box.
  const options = useMemo(() => {
    const all = nsList.data?.items.map((n) => n.namespace) ?? (currentNs ? [currentNs] : [])
    const set = new Set(all)
    const recentFirst = recents.filter((r) => r !== currentNs && set.has(r))
    const rest = all.filter((n) => n !== currentNs && !recentFirst.includes(n)).sort()
    const ordered = currentNs ? [currentNs, ...recentFirst, ...rest] : [...recentFirst, ...rest]
    return ordered.map((n) => ({ value: n, label: n }))
  }, [nsList.data, recents, currentNs])

  const switchTo = (nextNs: string) => {
    if (!nextNs || nextNs === currentNs) return
    navigate(`/ns/${encodeURIComponent(nextNs)}${subPath}`)
  }

  return (
    <Selector
      size="sm"
      label="Namespace"
      isLabelHidden
      width={220}
      placeholder={nsList.isLoading ? 'loading…' : 'Pick a namespace'}
      value={currentNs ?? ''}
      onChange={switchTo}
      options={options}
      hasSearch
      searchPlaceholder="Filter namespaces"
      emptyText="No namespaces yet"
      isLoading={nsList.isLoading}
    />
  )
}
