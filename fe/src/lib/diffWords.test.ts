import { describe, it, expect } from 'vitest'
import { diffWords, type DiffWord } from './diffWords'

const changed = (words: DiffWord[]) => words.filter(word => word.changed).map(word => word.text)
const text = (words: DiffWord[]) => words.map(word => word.text).join(' ')

describe('diffWords', () => {
  it('marks nothing when the answers match', () => {
    const diff = diffWords("I don't have time.", "I don't have time.")

    expect(changed(diff.user)).toEqual([])
    expect(changed(diff.correct)).toEqual([])
  })

  it('keeps every word of both answers, in order', () => {
    const diff = diffWords('I have no time.', "I don't have time.")

    expect(text(diff.user)).toBe('I have no time.')
    expect(text(diff.correct)).toBe("I don't have time.")
  })

  it('marks only the words that differ', () => {
    const diff = diffWords('I have no time.', "I don't have time.")

    expect(changed(diff.user)).toEqual(['no'])
    expect(changed(diff.correct)).toEqual(["don't"])
  })

  it('marks a missing word on the correct side only', () => {
    const diff = diffWords('I went school.', 'I went to school.')

    expect(changed(diff.user)).toEqual([])
    expect(changed(diff.correct)).toEqual(['to'])
  })

  it('marks an extra word on the user side only', () => {
    const diff = diffWords('I did go to school.', 'I go to school.')

    expect(changed(diff.user)).toEqual(['did'])
    expect(changed(diff.correct)).toEqual([])
  })

  it('ignores the final punctuation mark, which the grader accepts', () => {
    const diff = diffWords('Are you sure.', 'Are you sure?')

    expect(changed(diff.user)).toEqual([])
    expect(changed(diff.correct)).toEqual([])
  })

  it('ignores a missing final punctuation mark, which the grader accepts', () => {
    const diff = diffWords('I have time', 'I have time.')

    expect(changed(diff.user)).toEqual([])
    expect(changed(diff.correct)).toEqual([])
  })

  it('ignores a curly apostrophe, which the grader accepts', () => {
    const diff = diffWords('I don’t have time.', "I don't have time.")

    expect(changed(diff.user)).toEqual([])
    expect(changed(diff.correct)).toEqual([])
    expect(text(diff.user)).toBe('I don’t have time.')
  })

  it('marks punctuation that differs inside the sentence', () => {
    const diff = diffWords('Do you know. Yes, I do.', 'Do you know? Yes, I do.')

    expect(changed(diff.user)).toEqual(['know.'])
    expect(changed(diff.correct)).toEqual(['know?'])
  })

  it('ignores letter case, which the grader accepts', () => {
    const diff = diffWords('i have Time.', 'I have time.')

    expect(changed(diff.user)).toEqual([])
    expect(changed(diff.correct)).toEqual([])
  })

  it('ignores whitespace differences, which the grader accepts', () => {
    const diff = diffWords('  I  have\ntime. ', 'I have time.')

    expect(changed(diff.user)).toEqual([])
    expect(text(diff.user)).toBe('I have time.')
  })

  it('handles an empty answer', () => {
    const diff = diffWords('', 'I have time.')

    expect(diff.user).toEqual([])
    expect(changed(diff.correct)).toEqual(['I', 'have', 'time.'])
  })

  it('gives up on highlighting for an answer far longer than a sentence', () => {
    const long = Array.from({ length: 400 }, (_, i) => `word${i}`).join(' ')

    const diff = diffWords(long, 'I have time.')

    expect(changed(diff.user)).toEqual([])
    expect(changed(diff.correct)).toEqual([])
    expect(text(diff.correct)).toBe('I have time.')
  })
})
