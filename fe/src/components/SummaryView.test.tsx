import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import SummaryView from './SummaryView'
import type { Phrase } from '@/lib/api'

const phrases: Phrase[] = [
  {
    phrase: 'in the future',
    meaning_en: 'at some time later than now',
    example_en: 'I want to live abroad in the future.',
  },
]

function renderView(overrides = {}) {
  const props = {
    naturalEnglish:
      'I like dogs, especially Shiba Inu. I have a cat now, but I want a dog in the future.',
    naturalnessWhyEn: 'You opened every turn with "I think that", which reads as written English.',
    naturalnessFixEn: 'Drop "that" after "I think", and vary how you start a turn.',
    phrases,
    question: 'Who should take responsibility?',
    reflectionJa: '制度そのものを変えるべきだと思う。',
    transcript: [
      { role: 'user' as const, text: 'I think companies are responsible.' },
      { role: 'ai' as const, text: 'What makes you say that?' },
      { role: 'user' as const, text: 'Because they make the most impact.' },
    ],
    onRestart: vi.fn(),
    ...overrides,
  }
  render(<SummaryView {...props} />)
  return props
}

// The session itself is rendered by SessionSummary, which the history panel
// shares — see its tests for each card. What is checked here is that the
// summary screen shows the whole session and leads on from it.
describe('SummaryView', () => {
  it('shows the finished session', () => {
    renderView()
    expect(screen.getByText('Conversation')).toBeInTheDocument()
    expect(screen.getByText('Who should take responsibility?')).toBeInTheDocument()
    expect(screen.getByText('I think companies are responsible.')).toBeInTheDocument()
    expect(screen.getByText('What makes you say that?')).toBeInTheDocument()
    expect(screen.getByText('Because they make the most impact.')).toBeInTheDocument()
    expect(screen.getByText('制度そのものを変えるべきだと思う。')).toBeInTheDocument()
    expect(screen.getByText('Natural English')).toBeInTheDocument()
    expect(
      screen.getByText(
        'I like dogs, especially Shiba Inu. I have a cat now, but I want a dog in the future.'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('Why it sounded unnatural')).toBeInTheDocument()
    expect(screen.getByText('How to fix it')).toBeInTheDocument()
    expect(screen.getByText('Useful phrases')).toBeInTheDocument()
    expect(screen.getByText('in the future')).toBeInTheDocument()
  })

  // A learner who already said everything naturally gets no phrases, and the
  // screen still has to lead on to the next question.
  it('leads on to the next question even when there is nothing to pick up', () => {
    renderView({ phrases: [] })
    expect(screen.queryByText('Useful phrases')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next question' })).toBeInTheDocument()
  })

  it('starts a new discussion', () => {
    const props = renderView()
    fireEvent.click(screen.getByRole('button', { name: 'Next question' }))
    expect(props.onRestart).toHaveBeenCalledTimes(1)
  })

  it('links to the history page', () => {
    renderView()
    expect(screen.getByRole('link', { name: 'View history' })).toHaveAttribute(
      'href',
      '/discussion/history'
    )
  })
})
