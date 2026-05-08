import { useState, useRef, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useNamespacesOverview } from '../hooks/useNamespacesOverview'
import { useActiveNamespace } from '../context/useActiveNamespace'
import { STATUS_META } from '../pages/namespaces/statusMeta'
import Icon from './Icon'

function NsAvatar({ name, statusDot, size = 28 }: { name: string; statusDot?: string; size?: number }) {
  return (
    <span className="relative shrink-0" style={{ width: size, height: size }}>
      <span
        className="flex h-full w-full items-center justify-center rounded border border-default bg-subtle font-semibold uppercase leading-none text-secondary"
        style={{ fontSize: Math.round(size * 0.42) }}
      >
        {name[0]}
      </span>
      {statusDot && (
        <span
          className={`absolute -bottom-0.5 -right-0.5 size-2 rounded-full border-2 border-surface ${statusDot}`}
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

  const activeEntry = data?.items.find(n => n.config.namespace === namespace)
  const activeStatus = activeEntry?.status

  function handleSelect(ns: string) {
    setNamespace(ns)
    setOpen(false)
  }

  return (
    <div ref={ref} className="relative">
      {/* Trigger */}
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        aria-expanded={open}
        className={`flex w-full cursor-pointer items-center gap-2.5 rounded-lg border bg-surface px-3 py-2.5 text-left transition-colors duration-150 focus:outline-none focus:shadow-focus ${
          namespace
            ? 'border-default hover:border-strong'
            : 'border-default hover:border-strong'
        }`}
      >
        {namespace ? (
          <NsAvatar name={namespace} size={36} statusDot={activeStatus ? STATUS_META[activeStatus].dot : undefined} />
        ) : (
          <span className="flex size-9 shrink-0 items-center justify-center rounded border border-default bg-subtle text-muted">
            <Icon name="chevron-down" size={12} />
          </span>
        )}

        <span className="flex-1 min-w-0 text-left">
          {isLoading ? (
            <span className="block text-sm font-medium text-muted">Loading...</span>
          ) : namespace ? (
            <>
              <span className="block truncate text-sm font-semibold text-primary leading-snug">
                {namespace}
              </span>
            </>
          ) : (
            <span className="block text-sm font-medium text-muted">Select namespace...</span>
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
        <div className="absolute left-0 right-0 top-full z-50 mt-1.5 overflow-hidden rounded-lg border border-default bg-surface shadow-floating">
          {isLoading && (
            <p className="m-0 px-3 py-2.5 text-xs text-muted">Loading...</p>
          )}

          {!isLoading && (!data || data.items.length === 0) && (
            <p className="m-0 px-3 py-2.5 text-xs text-muted">No namespaces yet.</p>
          )}

          {data && data.items.length > 0 && (
            <ul className="m-0 max-h-60 list-none overflow-y-auto p-1">
              {data.items.map(({ config, status }) => {
                const isActive = config.namespace === namespace
                return (
                  <li key={config.namespace} className="m-0 p-0">
                    <button
                      type="button"
                      onClick={() => handleSelect(config.namespace)}
                      className={`flex w-full cursor-pointer items-center gap-2.5 rounded border px-2.5 py-2 transition-colors duration-100 focus-visible:outline-none focus-visible:shadow-focus ${
                        isActive
                          ? 'border-accent/20 bg-accent-subtle'
                          : 'border-transparent bg-transparent hover:bg-surface-raised'
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
              className="flex items-center gap-2 px-3 py-2 text-xs font-medium text-muted no-underline transition-colors duration-100 hover:bg-surface-raised hover:text-primary"
            >
              <span className="flex size-5 shrink-0 items-center justify-center rounded border border-default text-sm font-semibold leading-none">+</span>
              New namespace
            </Link>
          </div>
        </div>
      )}
    </div>
  )
}
