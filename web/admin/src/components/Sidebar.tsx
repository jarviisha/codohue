import NavLink from './NavLink'
import NamespacePicker from './NamespacePicker'
import Icon from './Icon'
import { logout } from '../services/api'
import { navRoutes } from '../routes'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { Button } from './ui'

export default function Sidebar() {
  const { namespace } = useActiveNamespace()

  async function handleLogout() {
    await logout().catch(() => null)
    window.location.href = '/login'
  }

  return (
    <nav
      aria-label="Main navigation"
      className="flex w-full shrink-0 flex-col border-b border-default bg-surface px-3 md:fixed md:left-0 md:top-0 md:h-screen md:w-64 md:border-b-0 md:border-r"
    >
      <div className="flex h-14 justify-between items-center px-3">
        <div className="min-w-0">
          <span className="block text-xl font-semibold leading-tight text-primary">
            @codohue
          </span>
        </div>
      </div>

      <div className="shrink-0 border-y border-default py-4">
        <p className="m-0 mb-2 px-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
          Namespace
        </p>
        <NamespacePicker />
        <div className="mt-2">
          <NavLink to="/namespaces" icon="world" end>Manage namespaces</NavLink>
        </div>
      </div>

      <div className="flex flex-1 flex-col overflow-y-auto py-4">
        <p className="m-0 mb-2 px-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted">
          Navigation
        </p>
        <div className="space-y-1">
          {navRoutes.map(route => (
            <NavLink key={route.path} to={`/${route.path}`} icon={route.icon}>{route.label}</NavLink>
          ))}
          {namespace && (
            <>
              <NavLink to={`/namespaces/${namespace}/catalog/items`} icon="image">Catalog</NavLink>
              <NavLink to={`/namespaces/${namespace}`} icon="settings" end>Settings</NavLink>
            </>
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
