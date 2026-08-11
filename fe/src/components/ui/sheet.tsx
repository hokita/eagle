'use client'

import * as React from 'react'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils'

interface SheetProps {
  open: boolean
  onClose: () => void
  title: string
  children: React.ReactNode
  className?: string
}

const FOCUSABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

// Elements inside the panel that can take focus, in DOM order. Controls that
// opt out of the tab order — Segmented's inactive tabs carry tabindex="-1" —
// are excluded, while visually hidden but focusable ones, like the level
// checkboxes the settings sheet renders at opacity 0, are included.
function getFocusable(root: HTMLElement | null): HTMLElement[] {
  if (!root) return []
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
}

export function Sheet({ open, onClose, title, children, className }: SheetProps) {
  const panelRef = React.useRef<HTMLDivElement>(null)
  const restoreFocusRef = React.useRef<HTMLElement | null>(null)

  React.useEffect(() => {
    if (!open) return
    const closeOnEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [open, onClose])

  // aria-modal="true" claims focus is confined to the dialog, so honour it:
  // pull focus in on open, and hand it back to whatever opened the sheet on
  // close. Without this the trigger keeps focus behind the overlay.
  React.useEffect(() => {
    if (!open) return
    restoreFocusRef.current = document.activeElement as HTMLElement | null
    const focusables = getFocusable(panelRef.current)
    ;(focusables[0] ?? panelRef.current)?.focus()
    return () => {
      restoreFocusRef.current?.focus()
    }
  }, [open])

  const trapTab = (e: React.KeyboardEvent) => {
    if (e.key !== 'Tab') return
    const focusables = getFocusable(panelRef.current)
    if (focusables.length === 0) {
      e.preventDefault()
      return
    }
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center">
      <div
        data-testid="sheet-backdrop"
        className="absolute inset-0 bg-black/25"
        onClick={onClose}
      />
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
        onKeyDown={trapTab}
        className={cn(
          'relative w-full max-w-2xl rounded-t-2xl border border-border bg-card p-5 pb-8 shadow-lg',
          className
        )}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold text-card-foreground">{title}</h2>
          <button
            type="button"
            aria-label="Close"
            onClick={onClose}
            className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  )
}
