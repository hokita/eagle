'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'

export interface SegmentedOption {
  value: string
  label: string
}

interface SegmentedProps {
  options: SegmentedOption[]
  value: string
  onChange: (value: string) => void
  label: string
  className?: string
}

export function Segmented({ options, value, onChange, label, className }: SegmentedProps) {
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return
    e.preventDefault()
    const index = options.findIndex(o => o.value === value)
    const delta = e.key === 'ArrowRight' ? 1 : -1
    const next = (index + delta + options.length) % options.length
    onChange(options[next].value)
  }

  return (
    <div
      role="tablist"
      aria-label={label}
      onKeyDown={handleKeyDown}
      className={cn('flex gap-1 rounded-lg border border-border bg-muted p-1', className)}
    >
      {options.map(option => {
        const selected = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            role="tab"
            aria-selected={selected}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(option.value)}
            className={cn(
              'flex-1 rounded-md px-2 py-1.5 text-xs transition-colors',
              selected
                ? 'bg-card font-semibold text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
