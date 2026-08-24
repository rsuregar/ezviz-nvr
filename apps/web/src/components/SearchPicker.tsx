import { useMemo, useState } from 'react'

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
export function SearchPicker({ placeholder, options, onSelect, emptyText }: Props) {
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    const pool = q
      ? options.filter((o) => o.label.toLowerCase().includes(q) || o.sublabel?.toLowerCase().includes(q))
      : options
    return pool.slice(0, 20)
  }, [options, query])

  return (
    <div className="relative">
      <input
        placeholder={placeholder}
        value={query}
        onChange={(e) => {
          setQuery(e.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 150)}
        className="w-full border border-slate-300 rounded px-3 py-2 text-sm"
      />
      {open && (
        <div className="absolute z-10 mt-1 w-full max-h-64 overflow-auto bg-white border border-slate-200 rounded-lg shadow-lg">
          {filtered.length === 0 && (
            <p className="px-3 py-2 text-sm text-slate-400">{emptyText ?? 'Tidak ada hasil'}</p>
          )}
          {filtered.map((opt) => (
            <button
              key={opt.id}
              type="button"
              onMouseDown={(e) => {
                e.preventDefault()
                onSelect(opt.id)
                setQuery('')
                setOpen(false)
              }}
              className="w-full text-left px-3 py-2 text-sm hover:bg-slate-50 flex flex-col"
            >
              <span className="text-slate-900">{opt.label}</span>
              {opt.sublabel && <span className="text-xs text-slate-400">{opt.sublabel}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
