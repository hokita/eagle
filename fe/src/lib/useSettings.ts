'use client'

import { useEffect, useState } from 'react'

export const LEVELS = [1, 2, 3, 4, 5]

export type ExplainLanguage = 'en' | 'ja'

export const SELECTED_LEVELS_STORAGE_KEY = 'eagle:selectedLevels'
export const EXPLAIN_LANGUAGE_STORAGE_KEY = 'eagle:explainLanguage'

export function loadStoredLevels(): number[] {
  if (typeof window === 'undefined') return LEVELS
  try {
    const raw = window.localStorage.getItem(SELECTED_LEVELS_STORAGE_KEY)
    if (!raw) return LEVELS
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed) && parsed.every((n): n is number => typeof n === 'number')) {
      return parsed
    }
  } catch {
    // ignore malformed storage, fall back to default
  }
  return LEVELS
}

export function loadStoredLanguage(): ExplainLanguage {
  if (typeof window === 'undefined') return 'en'
  return window.localStorage.getItem(EXPLAIN_LANGUAGE_STORAGE_KEY) === 'ja' ? 'ja' : 'en'
}

// levelsForRequest returns undefined when the selection carries no
// information — every level and no level both mean "any level" to the API.
export function levelsForRequest(levels: number[]): number[] | undefined {
  return levels.length === 0 || levels.length === LEVELS.length ? undefined : levels
}

export function toggleLevel(levels: number[], n: number): number[] {
  return levels.includes(n)
    ? levels.filter(l => l !== n)
    : [...levels, n].sort((a, b) => a - b)
}

export function levelSummary(levels: number[]): string {
  return levels.length === 0 || levels.length === LEVELS.length
    ? 'All'
    : [...levels].sort((a, b) => a - b).join(', ')
}

export function useSettings() {
  const [levels, setLevelsState] = useState<number[]>(LEVELS)
  const [language, setLanguageState] = useState<ExplainLanguage>('en')

  useEffect(() => {
    setLevelsState(loadStoredLevels())
    setLanguageState(loadStoredLanguage())
  }, [])

  const setLevels = (next: number[]) => {
    setLevelsState(next)
    window.localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, JSON.stringify(next))
  }

  const setLanguage = (next: ExplainLanguage) => {
    setLanguageState(next)
    window.localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, next)
  }

  return { levels, language, setLevels, setLanguage }
}
