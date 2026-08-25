import { useId, useMemo, useState } from 'react'

export interface PickerOption {
  id: string
  label: string
  sublabel?: string
}

interface Props {
  placeholder: string
  options: PickerOption[]
  onSelect: (id: string) => void
  emptyText?: string
}

// Type-to-filter picker: a plain text input that narrows a dropdown list of
// options as you type, instead of pasting a raw ID. Used for "assign camera
// to workspace" and "add member" once the option list can be large enough
// that a plain <select> stops being usable.
//
// Implements the ARIA combobox pattern (role="combobox" + a listbox of
// role="option"s, aria-activedescendant tracking the highlighted option) so
// it's usable from the keyboard, not just the mouse: Arrow keys move
// through results, Enter selects, Escape closes. Before this there was no
// way to reach a result without a pointer at all.
export function SearchPicker({ placeholder, options, onSelect, emptyText }: Props) {
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)
  const listId = useId()

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    const pool = q
      ? options.filter((o) => o.label.toLowerCase().includes(q) || o.sublabel?.toLowerCase().includes(q))
      : options
    return pool.slice(0, 20)
  }, [options, query])

  function select(opt: PickerOption) {
    onSelect(opt.id)
    setQuery('')
    setOpen(false)
    setActiveIndex(-1)
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setOpen(true)
      setActiveIndex((i) => Math.min(i + 1, filtered.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter') {
      if (open && activeIndex >= 0 && filtered[activeIndex]) {
        e.preventDefault()
        select(filtered[activeIndex])
      }
    } else if (e.key === 'Escape') {
      setOpen(false)
      setActiveIndex(-1)
    }
  }

  const activeOption = activeIndex >= 0 ? filtered[activeIndex] : undefined

  return (
    <div className="relative">
      <input
        role="combobox"
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={activeOption ? `${listId}-${activeOption.id}` : undefined}
        aria-label={placeholder}
        placeholder={placeholder}
        value={query}
        onChange={(e) => {
          setQuery(e.target.value)
          setOpen(true)
          setActiveIndex(-1)
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 150)}
        onKeyDown={onKeyDown}
        className="w-full border border-slate-300 rounded px-3 py-2 text-sm"
      />
      {open && (
        <div id={listId} role="listbox" className="absolute z-10 mt-1 w-full max-h-64 overflow-auto bg-white border border-slate-200 rounded-lg shadow-lg">
          {filtered.length === 0 && (
            <p className="px-3 py-2 text-sm text-slate-500">{emptyText ?? 'Tidak ada hasil'}</p>
          )}
          {filtered.map((opt, i) => (
            <button
              key={opt.id}
              id={`${listId}-${opt.id}`}
              role="option"
              aria-selected={i === activeIndex}
              type="button"
              onMouseEnter={() => setActiveIndex(i)}
              onMouseDown={(e) => {
                e.preventDefault()
                select(opt)
              }}
              className={`w-full text-left px-3 py-2 text-sm flex flex-col ${i === activeIndex ? 'bg-slate-50' : ''}`}
            >
              <span className="text-slate-900">{opt.label}</span>
              {opt.sublabel && <span className="text-xs text-slate-500">{opt.sublabel}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
