import { useState, useRef, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import { useActiveNamespace } from '../context/NamespaceContext'
import { STATUS_META } from '../pages/namespaces/statusMeta'
import Icon from './Icon'

const AVATAR_PALETTE = [
  '#6366F1', '#8B5CF6', '#EC4899', '#F59E0B',
  '#10B981', '#3B82F6', '#EF4444', '#14B8A6',
]

function nsAvatarColor(name: string): string {
  let h = 0
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) & 0xffffff
  return AVATAR_PALETTE[Math.abs(h) % AVATAR_PALETTE.length]
}

function NsAvatar({ name, statusDot, size = 28 }: { name: string; statusDot?: string; size?: number }) {
  return (
    <span className="relative shrink-0" style={{ width: size, height: size }}>
      <span
        className="flex items-center justify-center rounded font-bold text-white uppercase leading-none w-full h-full"
        style={{ background: nsAvatarColor(name), fontSize: Math.round(size * 0.46) }}
      >
        {name[0]}
      </span>
      {statusDot && (
        <span
          className={`absolute -bottom-0.5 -right-0.5 w-2 h-2 rounded-full border-2 border-base ${statusDot}`}
        />
      )}
    </span>
  )
}

export default function NamespacePicker() {
  const { namespace, setNamespace } = useActiveNamespace()
  const { data, isLoading } = useNamespacesOverview()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    function onMouseDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onMouseDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('mousedown', onMouseDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  const activeEntry = data?.namespaces.find(n => n.config.namespace === namespace)
  const activeStatus = activeEntry?.status

  function handleSelect(ns: string) {
    setNamespace(ns)
    setOpen(false)
  }

  return (
    <div ref={ref} className="relative">
      {/* Trigger */}
      <button
        onClick={() => setOpen(o => !o)}
        className={`w-full flex items-center gap-2.5 px-3 py-2.5 rounded border transition-colors duration-150 cursor-pointer focus:outline-none focus:shadow-focus ${
          namespace
            ? 'border-default hover:border-accent/60'
            : 'border-default hover:border-strong'
        }`}
      >
        {namespace ? (
          <NsAvatar name={namespace} size={36} statusDot={activeStatus ? STATUS_META[activeStatus].dot : undefined} />
        ) : (
          <span className="w-7.5 h-7.5 flex items-center justify-center rounded bg-surface-raised border border-default shrink-0 text-muted">
            <Icon name="chevron-down" size={12} />
          </span>
        )}

        <span className="flex-1 min-w-0 text-left">
          {isLoading ? (
            <span className="block font-medium text-muted font-mono">Loading…</span>
          ) : namespace ? (
            <>
              <span className="block truncate text-sm font-semibold text-primary leading-snug">
                {namespace}
              </span>
            </>
          ) : (
            <span className="block text-[13px] text-muted font-mono">Select namespace…</span>
          )}
        </span>

        <Icon
          name="chevron-down"
          size={12}
          className={`shrink-0 transition-transform duration-150 ${open ? 'rotate-180' : ''} ${namespace ? 'text-accent' : 'text-muted'}`}
        />
      </button>

      {/* Dropdown panel */}
      {open && (
        <div className="absolute top-full left-0 right-0 mt-1.5 bg-surface border border-default rounded shadow-floating overflow-hidden z-50">
          {isLoading && (
            <p className="px-3 py-2.5 text-xs text-muted m-0">Loading…</p>
          )}

          {!isLoading && (!data || data.namespaces.length === 0) && (
            <p className="px-3 py-2.5 text-xs text-muted m-0">No namespaces yet.</p>
          )}

          {data && data.namespaces.length > 0 && (
            <ul className="max-h-60 overflow-y-auto list-none m-0 p-0">
              {data.namespaces.map(({ config, status }) => {
                const isActive = config.namespace === namespace
                return (
                  <li key={config.namespace} className="m-0 p-0">
                    <button
                      onClick={() => handleSelect(config.namespace)}
                      className={`w-full flex items-center gap-2.5 px-3 py-2 cursor-pointer border-0 transition-colors duration-100 ${
                        isActive
                          ? 'bg-accent-subtle'
                          : 'bg-transparent hover:bg-surface-raised'
                      }`}
                    >
                      <NsAvatar name={config.namespace} size={24} />
                      <span className="flex-1 text-[13px] font-medium truncate text-left text-primary">
                        {config.namespace}
                      </span>
                      {isActive ? (
                        <Icon name="check" size={12} className="text-accent shrink-0" />
                      ) : (
                        <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${STATUS_META[status].dot}`} />
                      )}
                    </button>
                  </li>
                )
              })}
            </ul>
          )}

          <div className="border-t border-default">
            <Link
              to="/namespaces/new"
              onClick={() => setOpen(false)}
              className="flex items-center gap-2 px-3 py-2 text-xs font-medium text-muted hover:text-primary hover:bg-surface-raised no-underline transition-colors duration-100"
            >
              <span className="w-5 h-5 flex items-center justify-center rounded border border-default text-sm leading-none font-semibold shrink-0">+</span>
              New namespace
            </Link>
          </div>
        </div>
      )}
    </div>
  )
}
