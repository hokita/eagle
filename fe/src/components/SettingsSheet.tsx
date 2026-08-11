'use client'

import { Sheet } from '@/components/ui/sheet'
import { Segmented } from '@/components/ui/segmented'
import { LEVELS, toggleLevel, type ExplainLanguage } from '@/lib/useSettings'

interface SettingsSheetProps {
  open: boolean
  onClose: () => void
  levels: number[]
  onLevelsChange: (levels: number[]) => void
  language: ExplainLanguage
  onLanguageChange: (language: ExplainLanguage) => void
}

const LANGUAGE_OPTIONS = [
  { value: 'en', label: 'English' },
  { value: 'ja', label: '日本語' },
]

export default function SettingsSheet({
  open,
  onClose,
  levels,
  onLevelsChange,
  language,
  onLanguageChange,
}: SettingsSheetProps) {
  return (
    <Sheet open={open} onClose={onClose} title="Settings">
      <div className="mb-6">
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Sentence levels
        </div>
        <div className="flex flex-wrap gap-2">
          {LEVELS.map(n => {
            const checked = levels.includes(n)
            return (
              <label
                key={n}
                className={
                  checked
                    ? 'cursor-pointer rounded-full border border-primary bg-primary px-3 py-1 text-sm font-semibold text-primary-foreground'
                    : 'cursor-pointer rounded-full border border-border bg-card px-3 py-1 text-sm text-muted-foreground'
                }
              >
                <input
                  type="checkbox"
                  className="sr-only"
                  checked={checked}
                  aria-label={`Level ${n}`}
                  onChange={() => onLevelsChange(toggleLevel(levels, n))}
                />
                {n}
              </label>
            )
          })}
        </div>
      </div>

      <div>
        <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          AI language
        </div>
        <Segmented
          options={LANGUAGE_OPTIONS}
          value={language}
          onChange={value => onLanguageChange(value as ExplainLanguage)}
          label="AI language"
        />
        <p className="mt-2 text-xs text-muted-foreground">
          Used for explanations and weakness insight
        </p>
      </div>
    </Sheet>
  )
}
