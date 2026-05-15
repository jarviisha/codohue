import type { NamespaceFormErrors } from '../configForm'
import { TAB_LABEL, tabHasError } from './tabs'
import type { TabId } from './types'

interface TabNavProps {
  tabs: TabId[]
  active: TabId
  errors: NamespaceFormErrors
  onSelect: (tab: TabId) => void
}

// Vertical-on-large, horizontal-scroll-on-small tab nav used inside the
// namespace config form. Carries an error indicator on tabs whose fields
// have validation messages after first submit.
export default function TabNav({ tabs, active, errors, onSelect }: TabNavProps) {
  return (
    <div
      role="tablist"
      aria-label="Namespace config sections"
      aria-orientation="vertical"
      className="flex flex-row lg:flex-col gap-1 overflow-x-auto lg:overflow-visible border-b lg:border-b-0 border-default pb-2 lg:pb-0"
    >
      {tabs.map((tab) => {
        const selected = tab === active
        const invalid = tabHasError(tab, errors)
        return (
          <button
            key={tab}
            type="button"
            role="tab"
            aria-selected={selected}
            aria-controls={`ns-config-${tab}`}
            onClick={() => onSelect(tab)}
            className={[
              'h-9 px-3 rounded-sm font-mono text-xs uppercase tracking-[0.04em] border border-transparent text-left shrink-0',
              selected
                ? 'bg-accent-subtle text-accent border-default'
                : 'text-secondary hover:bg-surface-raised hover:text-primary',
              invalid ? 'text-danger' : '',
            ].join(' ')}
          >
            {TAB_LABEL[tab]}
          </button>
        )
      })}
    </div>
  )
}
