import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import QuestionCard from './QuestionCard'
import type { Sentence } from '@/lib/api'

const sentence: Sentence = {
  id: 1,
  japanese: '時間がありません。',
  english: "I don't have time.",
  page: '12',
  level: 2,
  correct_count: 5,
  incorrect_count: 2,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

function renderCard(overrides = {}) {
  const props = {
    sentence,
    correctCount: 5,
    incorrectCount: 2,
    levelSummary: 'All',
    isSpeaking: false,
    onSpeak: vi.fn(),
    ...overrides,
  }
  render(<QuestionCard {...props} />)
  return props
}

describe('QuestionCard', () => {
  it('shows the japanese sentence', () => {
    renderCard()
    expect(screen.getByText('時間がありません。')).toBeInTheDocument()
  })

  it('shows the counters as their own exact strings', () => {
    renderCard()
    expect(screen.getByText(/^Correct: 5$/)).toBeInTheDocument()
    expect(screen.getByText(/^Incorrect: 2$/)).toBeInTheDocument()
  })

  it('shows the active level summary', () => {
    renderCard({ levelSummary: '1, 3' })
    expect(screen.getByText('Level: 1, 3')).toBeInTheDocument()
  })

  it('speaks when the listen button is clicked', () => {
    const props = renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Listen' }))
    expect(props.onSpeak).toHaveBeenCalledTimes(1)
  })

  it('disables the listen button while speaking', () => {
    renderCard({ isSpeaking: true })
    expect(screen.getByRole('button', { name: 'Listen' })).toBeDisabled()
  })

  it('never reveals the english answer', () => {
    renderCard()
    expect(screen.queryByText("I don't have time.")).not.toBeInTheDocument()
  })
})
