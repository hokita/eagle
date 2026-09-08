import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import SessionSummary from './SessionSummary'
import type { Phrase } from '@/lib/api'

const phrases: Phrase[] = [
  {
    phrase: 'in the future',
    meaning_en: 'at some time later than now',
    example_en: 'I want to live abroad in the future.',
  },
]

function renderSummary(overrides = {}) {
  const props = {
    question: 'Who should take responsibility?',
    transcript: [
      { role: 'user' as const, text: 'I think companies are responsible.' },
      { role: 'ai' as const, text: 'What makes you say that?' },
    ],
    reflectionJa: '制度そのものを変えるべきだと思う。',
    naturalEnglish: 'I think companies are responsible, and the system should change.',
    naturalnessWhyEn: 'You opened every turn with "I think that".',
    naturalnessFixEn: 'Vary how you start a turn.',
    phrases,
    ...overrides,
  }
  render(<SessionSummary {...props} />)
  return props
}

describe('SessionSummary', () => {
  it('shows the question and the whole conversation', () => {
    renderSummary()
    expect(screen.getByText('Conversation')).toBeInTheDocument()
    expect(screen.getByText('Who should take responsibility?')).toBeInTheDocument()
    expect(screen.getByText('I think companies are responsible.')).toBeInTheDocument()
    expect(screen.getByText('What makes you say that?')).toBeInTheDocument()
  })

  // The rewrite claims to include the ideas that stayed in Japanese, which is
  // only checkable with the Japanese on the same screen.
  it('shows the Japanese reflection', () => {
    renderSummary()
    expect(screen.getByText('Reflection')).toBeInTheDocument()
    expect(screen.getByText('制度そのものを変えるべきだと思う。')).toBeInTheDocument()
  })

  it('shows the natural English rewrite with what it is', () => {
    renderSummary()
    expect(screen.getByText('Natural English')).toBeInTheDocument()
    expect(
      screen.getByText('Everything you said, the way a native speaker would say it.')
    ).toBeInTheDocument()
    expect(
      screen.getByText('I think companies are responsible, and the system should change.')
    ).toBeInTheDocument()
  })

  it('explains why the English sounded unnatural and how to fix it', () => {
    renderSummary()
    expect(screen.getByText('Why it sounded unnatural')).toBeInTheDocument()
    expect(screen.getByText('You opened every turn with "I think that".')).toBeInTheDocument()
    expect(screen.getByText('How to fix it')).toBeInTheDocument()
    expect(screen.getByText('Vary how you start a turn.')).toBeInTheDocument()
  })

  it('lists each phrase with its meaning and example', () => {
    renderSummary()
    expect(screen.getByText('Useful phrases')).toBeInTheDocument()
    expect(screen.getByText('in the future')).toBeInTheDocument()
    expect(screen.getByText('at some time later than now')).toBeInTheDocument()
    expect(screen.getByText('I want to live abroad in the future.')).toBeInTheDocument()
  })

  // Sessions saved before the summary replaced the study/retry flow read back
  // with these fields empty; a titled card holding nothing looks like a load
  // that failed rather than a session that predates the feature.
  it('hides every card a session has nothing for, keeping the conversation', () => {
    renderSummary({
      reflectionJa: '',
      naturalEnglish: '',
      naturalnessWhyEn: '',
      naturalnessFixEn: '',
      phrases: [],
    })
    expect(screen.queryByText('Reflection')).not.toBeInTheDocument()
    expect(screen.queryByText('Natural English')).not.toBeInTheDocument()
    expect(screen.queryByText('Why it sounded unnatural')).not.toBeInTheDocument()
    expect(screen.queryByText('How to fix it')).not.toBeInTheDocument()
    expect(screen.queryByText('Useful phrases')).not.toBeInTheDocument()
    expect(screen.getByText('Conversation')).toBeInTheDocument()
    expect(screen.getByText('I think companies are responsible.')).toBeInTheDocument()
  })
})
