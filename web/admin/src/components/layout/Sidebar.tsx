import { useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import SidebarNavGroup from './SidebarNavGroup'
import SidebarNavItem from './SidebarNavItem'
import StatusToken, { type StatusState } from '@/components/ui/StatusToken'
import { namespaceNav, parsePs1 } from '@/routes/nav'
import { paths } from '@/routes/path'
import { probeState, useHealth } from '@/services/health'
import { getLastNamespace, setLastNamespace } from '@/utils/lastNamespace'

// Fixed left sidebar. Two sections: GLOBAL (always shown) and {namespace}
// (shown when current route is /ns/:name/...). See DESIGN.md §3.1.
//
// The Health nav row carries an inline live StatusToken so the operator sees
// system health from any page without leaving their current view.
export default function Sidebar() {
  const { pathname } = useLocation()
  const { ns } = parsePs1(pathname)
  const urlNs = ns === '~' ? undefined : ns
  // Sticky last-namespace memory: when the URL is a global page (Health,
  // Namespaces, Demo Data, Danger Zone) we still surface the namespace nav
  // for whichever namespace the operator was on most recently in this tab.
  // Items show no `aria-current` highlight on global routes, so the nav
  // reads as "remembered context", not "active".
  const activeNs = urlNs ?? getLastNamespace() ?? undefined
  useEffect(() => {
    if (urlNs) setLastNamespace(urlNs)
  }, [urlNs])
  const { data: health, isLoading: healthLoading } = useHealth()

  const healthState: StatusState = healthLoading
    ? 'idle'
    : !health
      ? 'idle'
      : probeState(health.status) === 'ok'
        ? 'ok'
        : 'warn'

  return (
    <aside
      aria-label="Primary navigation"
      className="fixed left-0 top-0 h-screen w-60 bg-base border-r border-default flex flex-col z-40"
    >
      <div className="h-12 px-4 flex items-center border-b border-default">
        <span className="font-mono text-xs font-semibold uppercase tracking-[0.12em] text-primary">
          codohue
        </span>
      </div>

      <div className="flex-1 overflow-y-auto">
        <SidebarNavGroup label="Global">
          <SidebarNavItem
            to={paths.health}
            end
            trailing={<StatusToken state={healthState} />}
          >
            Health
          </SidebarNavItem>
          <SidebarNavItem to={paths.namespaces} end>
            Namespaces
          </SidebarNavItem>
          <SidebarNavItem to={paths.demoData} end>
            Demo Data
          </SidebarNavItem>
          <SidebarNavItem to={paths.dangerZone} end>
            Danger Zone
          </SidebarNavItem>
        </SidebarNavGroup>

        {activeNs ? (
          <SidebarNavGroup label={activeNs}>
            {namespaceNav(activeNs).map((item) => (
              <SidebarNavItem key={item.to} to={item.to} end={item.end}>
                {item.label}
              </SidebarNavItem>
            ))}
          </SidebarNavGroup>
        ) : null}
      </div>
    </aside>
  )
}
