import NavLink from './NavLink'
import NamespacePicker from './NamespacePicker'
import Icon from './Icon'
import { logout } from '../services/api'
import { globalNavRoutes, namespaceNavRoutes } from '../routes'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { Button } from './ui'
import { appLayoutClasses } from './appLayoutClasses'

function namespacePath(path: string, namespace: string) {
  return `/${path.replace(':ns', encodeURIComponent(namespace))}`
}

export default function Sidebar() {
  const { namespace } = useActiveNamespace()

  async function handleLogout() {
    await logout().catch(() => null)
    window.location.href = '/login'
  }

  return (
    <nav
      aria-label="Main navigation"
      className={appLayoutClasses.sidebar}
    >
      <div className="flex h-12 justify-between items-center px-2">
        <div className="min-w-0">
          <span className="block text-lg font-semibold leading-tight text-primary">
            @codohue
          </span>
        </div>
      </div>

      <div className="shrink-0 border-y border-default py-3">
        <p className="m-0 mb-2 px-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
          Current Namespace
        </p>
        <NamespacePicker />
      </div>

      <div className="flex flex-1 flex-col overflow-y-auto py-3">
        <p className="m-0 mb-2 px-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
          Global
        </p>
        <div className="space-y-1">
          {globalNavRoutes.map(route => (
            <NavLink key={route.path} to={`/${route.path}`} icon={route.icon} end>
              {route.label}
            </NavLink>
          ))}
        </div>

        <p className="m-0 mb-2 mt-4 px-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
          Namespace Tools
        </p>
        <div className="space-y-1">
          {namespace && (
            namespaceNavRoutes.map(route => (
              <NavLink
                key={route.path}
                to={namespacePath(route.path, namespace)}
                icon={route.icon}
                end
              >
                {route.label}
              </NavLink>
            ))
          )}
          {!namespace && (
            <p className="m-0 rounded border border-default bg-subtle px-3 py-2 text-xs text-muted">
              Select a namespace to view operational tools.
            </p>
          )}
        </div>
      </div>

      <div className="border-t border-default py-3">
        <Button
          size="sm"
          variant="danger"
          onClick={handleLogout}
          className='gap-2 w-full'
        >
          <span>Logout</span>
          <Icon name="logout" size={14} />
        </Button>
      </div>
    </nav>
  )
}
