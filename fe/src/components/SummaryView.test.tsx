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
    onRestart: vi.fn(),
    ...overrides,
  }
  render(<SummaryView {...props} />)
  return props
}

describe('SummaryView', () => {
  it('shows the natural English rewrite', () => {
    renderView()
    expect(screen.getByText('Natural English')).toBeInTheDocument()
    expect(
      screen.getByText(
        'I like dogs, especially Shiba Inu. I have a cat now, but I want a dog in the future.'
      )
    ).toBeInTheDocument()
  })

  it('explains why the English sounded unnatural and how to fix it', () => {
    renderView()
    expect(screen.getByText('Why it sounded unnatural')).toBeInTheDocument()
    expect(
      screen.getByText('You opened every turn with "I think that", which reads as written English.')
    ).toBeInTheDocument()
    expect(screen.getByText('How to fix it')).toBeInTheDocument()
    expect(
      screen.getByText('Drop "that" after "I think", and vary how you start a turn.')
    ).toBeInTheDocument()
  })

  // Sessions saved before the explanation existed read back with both fields
  // empty; an empty card would look like a failed load.
  it('hides the explanation card for a session that has none', () => {
    renderView({ naturalnessWhyEn: '', naturalnessFixEn: '' })
    expect(screen.queryByText('Why it sounded unnatural')).not.toBeInTheDocument()
    expect(screen.queryByText('How to fix it')).not.toBeInTheDocument()
  })

  it('lists each phrase with its meaning and example', () => {
    renderView()
    expect(screen.getByText('Useful phrases')).toBeInTheDocument()
    expect(screen.getByText('in the future')).toBeInTheDocument()
    expect(screen.getByText('at some time later than now')).toBeInTheDocument()
    expect(screen.getByText('I want to live abroad in the future.')).toBeInTheDocument()
  })

  // A learner who already said everything naturally gets no phrases, and the
  // screen still has to lead on to the next question.
  it('hides the phrase card when there is nothing to pick up', () => {
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
