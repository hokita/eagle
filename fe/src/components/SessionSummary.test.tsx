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
      { role: 'user' as const, text: 'Because they make the most impact.' },
    ],
    reflectionJa: '制度そのものを変えるべきだと思う。',
    refinedMessages: ["I think it's on the companies.", 'Because they have the biggest impact.'],
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

  // The phrases start from the ideas that stayed in Japanese, which is only
  // checkable with the Japanese on the same screen.
  it('shows the Japanese reflection', () => {
    renderSummary()
    expect(screen.getByText('Reflection')).toBeInTheDocument()
    expect(screen.getByText('制度そのものを変えるべきだと思う。')).toBeInTheDocument()
  })

  // Each refinement is only readable against the exact words it refines, so
  // it sits directly under its own message inside the conversation — not in
  // a section of its own, and not as one merged passage.
  it('shows each of your messages refined, directly under it', () => {
    renderSummary()
    expect(
      screen.getByText('Under each of your messages: how a native speaker would say it.')
    ).toBeInTheDocument()
    expect(screen.queryByText('Natural English')).not.toBeInTheDocument()

    const first = screen.getByText("I think it's on the companies.")
    const second = screen.getByText('Because they have the biggest impact.')
    expect(first.closest('.text-right')).toHaveTextContent('I think companies are responsible.')
    expect(second.closest('.text-right')).toHaveTextContent('Because they make the most impact.')
    expect(first.closest('.text-right')).not.toHaveTextContent('What makes you say that?')
    expect(screen.getAllByText('More natural')).toHaveLength(2)
  })

  // The coach returns a turn unchanged when it already sounded natural;
  // repeating it would read as a correction that changed nothing.
  it('does not repeat a message whose refinement is unchanged', () => {
    renderSummary({
      refinedMessages: ['I think companies are responsible.', 'Because they have the biggest impact.'],
    })
    expect(screen.getAllByText('I think companies are responsible.')).toHaveLength(1)
    expect(screen.getAllByText('More natural')).toHaveLength(1)
    expect(screen.getByText('Because they have the biggest impact.')).toBeInTheDocument()
  })

  // When every turn already sounded natural nothing shows under any message,
  // so the line introducing the refinements would point at nothing.
  it('drops the refinement hint when every refinement is unchanged', () => {
    renderSummary({
      refinedMessages: ['I think companies are responsible.', 'Because they make the most impact.'],
    })
    expect(screen.queryByText('More natural')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Under each of your messages: how a native speaker would say it.')
    ).not.toBeInTheDocument()
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
      refinedMessages: [],
      naturalnessWhyEn: '',
      naturalnessFixEn: '',
      phrases: [],
    })
    expect(screen.queryByText('Reflection')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Under each of your messages: how a native speaker would say it.')
    ).not.toBeInTheDocument()
    expect(screen.queryByText('More natural')).not.toBeInTheDocument()
    expect(screen.queryByText('Why it sounded unnatural')).not.toBeInTheDocument()
    expect(screen.queryByText('How to fix it')).not.toBeInTheDocument()
    expect(screen.queryByText('Useful phrases')).not.toBeInTheDocument()
    expect(screen.getByText('Conversation')).toBeInTheDocument()
    expect(screen.getByText('I think companies are responsible.')).toBeInTheDocument()
  })
})
