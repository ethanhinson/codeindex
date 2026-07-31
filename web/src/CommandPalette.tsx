import { useEffect, useRef, useState } from 'react'

interface Props {
  onSubmit: (focusId: string) => void
  suggestions?: string[]
}

// A search box that sets the graph focus by id (sym:Foo, dec-…, itm-…,
// note-…, path:…). ⌘K or "/" focuses it.
export function CommandPalette({ onSubmit, suggestions = [] }: Props) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [value, setValue] = useState('')

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const meta = (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k'
      const slash = e.key === '/' && document.activeElement !== inputRef.current
      if (meta || slash) {
        e.preventDefault()
        inputRef.current?.focus()
        inputRef.current?.select()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  function submit(v: string) {
    const t = v.trim()
    if (t) onSubmit(t)
  }

  return (
    <div className="palette">
      <input
        ref={inputRef}
        className="palette-input"
        data-testid="palette-input"
        placeholder="Focus a node — sym:Name, dec-…, itm-…, path:…   ( / )"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') submit(value)
        }}
      />
      {suggestions.length > 0 && (
        <div className="palette-suggestions">
          {suggestions.map((s) => (
            <button key={s} className="chip" data-testid="suggestion" onClick={() => submit(s)}>
              {s}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
