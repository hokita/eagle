import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import {
  LEVELS,
  SELECTED_LEVELS_STORAGE_KEY,
  EXPLAIN_LANGUAGE_STORAGE_KEY,
  loadStoredLevels,
  loadStoredLanguage,
  levelsForRequest,
  toggleLevel,
  levelSummary,
  useSettings,
} from './useSettings'

beforeEach(() => {
  localStorage.clear()
})

describe('loadStoredLevels', () => {
  it('defaults to every level when nothing is stored', () => {
    expect(loadStoredLevels()).toEqual(LEVELS)
  })

  it('restores a stored selection', () => {
    localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, JSON.stringify([2, 3]))
    expect(loadStoredLevels()).toEqual([2, 3])
  })

  it('falls back to every level when the stored value is malformed JSON', () => {
    localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, 'not json')
    expect(loadStoredLevels()).toEqual(LEVELS)
  })

  it('falls back to every level when the stored value is not an array of numbers', () => {
    localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, JSON.stringify(['1', '2']))
    expect(loadStoredLevels()).toEqual(LEVELS)
  })
})

describe('loadStoredLanguage', () => {
  it('defaults to English', () => {
    expect(loadStoredLanguage()).toBe('en')
  })

  it('restores a stored Japanese preference', () => {
    localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, 'ja')
    expect(loadStoredLanguage()).toBe('ja')
  })

  it('treats any unrecognised stored value as English', () => {
    localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, 'fr')
    expect(loadStoredLanguage()).toBe('en')
  })
})

describe('levelsForRequest', () => {
  it('sends no filter when every level is selected', () => {
    expect(levelsForRequest(LEVELS)).toBeUndefined()
  })

  it('sends no filter when no level is selected', () => {
    expect(levelsForRequest([])).toBeUndefined()
  })

  it('sends the selection when it is a strict subset', () => {
    expect(levelsForRequest([2, 3])).toEqual([2, 3])
  })
})

describe('toggleLevel', () => {
  it('removes a selected level', () => {
    expect(toggleLevel([1, 2, 3], 2)).toEqual([1, 3])
  })

  it('adds an unselected level in ascending order', () => {
    expect(toggleLevel([1, 4], 2)).toEqual([1, 2, 4])
  })

  it('does not mutate the input', () => {
    const levels = [1, 2, 3]
    toggleLevel(levels, 2)
    expect(levels).toEqual([1, 2, 3])
  })
})

describe('levelSummary', () => {
  it('reads All when every level is selected', () => {
    expect(levelSummary(LEVELS)).toBe('All')
  })

  it('reads All when no level is selected, because that also means any level', () => {
    expect(levelSummary([])).toBe('All')
  })

  it('lists a subset in ascending order', () => {
    expect(levelSummary([3, 1])).toBe('1, 3')
  })
})

describe('useSettings', () => {
  it('hydrates both preferences from storage', () => {
    localStorage.setItem(SELECTED_LEVELS_STORAGE_KEY, JSON.stringify([2]))
    localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, 'ja')

    const { result } = renderHook(() => useSettings())

    expect(result.current.levels).toEqual([2])
    expect(result.current.language).toBe('ja')
  })

  it('persists levels when they change', () => {
    const { result } = renderHook(() => useSettings())

    act(() => result.current.setLevels([1, 2]))

    expect(result.current.levels).toEqual([1, 2])
    expect(localStorage.getItem(SELECTED_LEVELS_STORAGE_KEY)).toBe('[1,2]')
  })

  it('persists the language when it changes', () => {
    const { result } = renderHook(() => useSettings())

    act(() => result.current.setLanguage('ja'))

    expect(result.current.language).toBe('ja')
    expect(localStorage.getItem(EXPLAIN_LANGUAGE_STORAGE_KEY)).toBe('ja')
  })
})
