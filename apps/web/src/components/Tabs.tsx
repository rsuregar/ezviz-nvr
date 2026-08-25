import { type KeyboardEvent, type ReactNode } from 'react'

interface TabDef<T extends string> {
  id: T
  label: string
}

// WAI-ARIA tabs pattern: role="tablist"/"tab"/"tabpanel", roving tabindex
// (only the active tab is a Tab stop; Left/Right arrows move between tabs
// and activate them), and each tab<->panel pair cross-linked via
// aria-controls/aria-labelledby. Used by both the Admin and Workspace
// detail pages, which previously used plain unlabeled buttons — a screen
// reader had no way to know they were tabs, how many there were, or which
// one was selected.
export function TabList<T extends string>({
  tabs,
  active,
  onChange,
  idPrefix,
  label,
}: {
  tabs: TabDef<T>[]
  active: T
  onChange: (id: T) => void
  idPrefix: string
  label: string
}) {
  function onKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return
    e.preventDefault()
    const dir = e.key === 'ArrowRight' ? 1 : -1
    const next = tabs[(index + dir + tabs.length) % tabs.length]
    onChange(next.id)
    document.getElementById(`${idPrefix}-tab-${next.id}`)?.focus()
  }

  return (
    <div role="tablist" aria-label={label} className="flex gap-1 border-b border-slate-200">
      {tabs.map((t, i) => (
        <button
          key={t.id}
          id={`${idPrefix}-tab-${t.id}`}
          role="tab"
          type="button"
          aria-selected={active === t.id}
          aria-controls={`${idPrefix}-panel-${t.id}`}
          tabIndex={active === t.id ? 0 : -1}
          onClick={() => onChange(t.id)}
          onKeyDown={(e) => onKeyDown(e, i)}
          className={`px-3 py-2 text-sm font-medium border-b-2 -mb-px ${
            active === t.id ? 'border-slate-900 text-slate-900' : 'border-transparent text-slate-500'
          }`}
        >
          {t.label}
        </button>
      ))}
    </div>
  )
}

export function TabPanel({ id, tabId, active, children }: { id: string; tabId: string; active: boolean; children: ReactNode }) {
  if (!active) return null
  return (
    <div role="tabpanel" id={id} aria-labelledby={tabId} tabIndex={0}>
      {children}
    </div>
  )
}
