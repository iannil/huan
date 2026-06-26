import { useEffect, useCallback, useRef } from 'react'

type KeyCombo = string // e.g. 'j', 'Ctrl+s', 'Escape', 'g c'

interface UseKeyboardOptions {
  bindings: Record<KeyCombo, () => void>
  /** When true, ignore keyboard events (e.g. when typing in an input) */
  enabled?: boolean
}

/**
 * Simple keyboard shortcut hook for terminal-style navigation.
 *
 * Supports single keys (j, k, Enter, Escape, /, ?)
 * and Ctrl+key combos (Ctrl+s, Ctrl+q).
 *
 * Sequential combos like 'g c' use a 500ms window.
 */
export function useKeyboard({ bindings, enabled = true }: UseKeyboardOptions) {
  const bindingsRef = useRef(bindings)
  bindingsRef.current = bindings

  const seqRef = useRef('')
  const seqTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (!enabled) return

      // Ignore when user is typing in input/textarea (unless the binding is Escape/Enter)
      const target = e.target as HTMLElement
      const isInput =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable

      if (isInput && e.key !== 'Escape' && e.key !== 'Enter') return

      // Build combo string
      let combo = ''
      if (e.ctrlKey || e.metaKey) combo += 'Ctrl+'
      if (e.shiftKey && e.key !== 'Shift') combo += 'Shift+'
      if (e.key !== 'Control' && e.key !== 'Shift' && e.key !== 'Meta') {
        combo += e.key.length === 1 ? e.key.toLowerCase() : e.key
      }

      // Check exact match first
      if (bindingsRef.current[combo]) {
        e.preventDefault()
        bindingsRef.current[combo]()
        clearSeq()
        return
      }

      // Sequential combos (e.g. 'g c' → type 'g' then 'c')
      if (combo.length === 1) {
        const candidate = (seqRef.current + combo)
        if (bindingsRef.current[candidate]) {
          e.preventDefault()
          bindingsRef.current[candidate]()
          clearSeq()
          return
        }
        // Start/reset sequence buffer
        seqRef.current = combo
        if (seqTimerRef.current) clearTimeout(seqTimerRef.current)
        seqTimerRef.current = setTimeout(() => clearSeq(), 500)
      }
    },
    [enabled]
  )

  const clearSeq = () => {
    seqRef.current = ''
    if (seqTimerRef.current) {
      clearTimeout(seqTimerRef.current)
      seqTimerRef.current = null
    }
  }

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
      clearSeq()
    }
  }, [handleKeyDown])
}

/**
 * Format shortcut string for display (e.g. "Ctrl+S" → "[Ctrl+S]").
 */
export function shortcut(label: string): string {
  return `[${label}]`
}
