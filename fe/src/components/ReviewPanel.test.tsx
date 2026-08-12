import { render, screen, fireEvent, within } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ReviewPanel from './ReviewPanel'
import { answerRow, answerRows, highlightedWords } from '@/test-helpers'

const histories = [
  { id: 1, incorrect_answer: 'There is no time.', created_at: '2026-01-01T00:00:00Z' },
  { id: 2, incorrect_answer: 'I have not time.', created_at: '2026-01-02T00:00:00Z' },
]

function renderPanel(overrides = {}) {
  const props = {
    feedback: 'incorrect' as const,
    userAnswer: 'I have no time.',
    correctAnswer: "I don't have time.",
    histories: [],
    explanation: null,
    explaining: false,
    explainError: null,
    onExplain: vi.fn(),
    ...overrides,
  }
  render(<ReviewPanel {...props} />)
  return props
}

describe('verdict', () => {
  it('shows the incorrect verdict copy', () => {
    renderPanel({ feedback: 'incorrect' })
    expect(screen.getByText('Not quite right. Try again!')).toBeInTheDocument()
  })

  it('shows the correct verdict copy', () => {
    renderPanel({ feedback: 'correct' })
    expect(screen.getByText('Correct! Well done!')).toBeInTheDocument()
  })
})

describe('sections', () => {
  it('shows the answer, the previous attempts and the explanation together', () => {
    renderPanel({ histories, explanation: 'Prefer do-support here.' })

    expect(answerRow("I don't have time.")).toBeInTheDocument()
    expect(answerRow('There is no time.')).toBeInTheDocument()
    expect(screen.getByText('Prefer do-support here.')).toBeInTheDocument()
  })

  it('orders the panels answer, explanation, previous attempts', () => {
    renderPanel({ histories, explanation: 'Prefer do-support here.' })

    const labels = screen
      .getAllByText(/^(You wrote|Explanation|Previous attempts)/)
      .map(node => node.textContent)

    expect(labels).toEqual(['You wrote', 'Explanation', 'Previous attempts (2)'])
  })

  it('never renders a tab control', () => {
    renderPanel({ histories, explanation: 'Prefer do-support here.' })

    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
  })

  it('omits the attempts section when there is no history', () => {
    renderPanel({ histories: [] })

    expect(screen.queryByText(/^Previous attempts/)).not.toBeInTheDocument()
  })

  it('counts the previous attempts', () => {
    renderPanel({ histories })

    expect(screen.getByText('Previous attempts (2)')).toBeInTheDocument()
  })

  it('omits the explanation section when correct', () => {
    renderPanel({ feedback: 'correct', histories: [] })

    expect(screen.queryByText('Explanation')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Explain' })).not.toBeInTheDocument()
    expect(answerRow("I don't have time.")).toBeInTheDocument()
  })
})

describe('answer section', () => {
  it('shows the correct answer', () => {
    renderPanel()
    expect(answerRow("I don't have time.")).toBeInTheDocument()
  })

  it('shows what the user wrote when they were wrong', () => {
    renderPanel({ feedback: 'incorrect' })
    expect(answerRow('I have no time.')).toBeInTheDocument()
  })

  it('does not repeat the user answer when they were right', () => {
    renderPanel({ feedback: 'correct', userAnswer: "I don't have time." })
    expect(screen.queryByText('You wrote')).not.toBeInTheDocument()
  })

  it('puts the correct answer directly below what the user wrote, with nothing between', () => {
    renderPanel({ feedback: 'incorrect' })

    const wrong = answerRow('I have no time.')
    const right = answerRow("I don't have time.")
    // Only the "Correct" label separates the two answers.
    expect(wrong.nextElementSibling?.textContent).toBe('Correct')
    expect(wrong.nextElementSibling?.nextElementSibling).toBe(right)
  })

  it('highlights the words that differ on both answers', () => {
    renderPanel({ feedback: 'incorrect' })

    expect(highlightedWords('I have no time.')).toEqual(['no'])
    expect(highlightedWords("I don't have time.")).toEqual(["don't"])
  })

  it('highlights nothing on the correct answer when there is nothing to compare', () => {
    renderPanel({ feedback: 'correct', userAnswer: "I don't have time." })

    expect(highlightedWords("I don't have time.")).toEqual([])
  })

  it('renders each answer once, so the two rows stay a single comparison', () => {
    renderPanel({ feedback: 'incorrect' })

    expect(answerRows('I have no time.')).toHaveLength(1)
    expect(answerRows("I don't have time.")).toHaveLength(1)
  })
})

describe('attempts section', () => {
  it('lists every previous incorrect answer', () => {
    renderPanel({ histories })

    expect(answerRow('There is no time.')).toBeInTheDocument()
    expect(answerRow('I have not time.')).toBeInTheDocument()
  })

  it('highlights how each previous attempt differs from the correct answer', () => {
    renderPanel({ histories })

    expect(highlightedWords('There is no time.')).toEqual(['There', 'is', 'no'])
    expect(highlightedWords('I have not time.')).toEqual(['not'])
  })
})

describe('explanation section', () => {
  it('requests the explanation from the Explain button', () => {
    const props = renderPanel({ feedback: 'incorrect' })

    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))

    expect(props.onExplain).toHaveBeenCalledTimes(1)
  })

  it('shows a loading state while fetching, in place of the button', () => {
    renderPanel({ explaining: true })

    expect(screen.getByText('Explaining...')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Explain' })).not.toBeInTheDocument()
  })

  it('replaces the button with the explanation once it arrives', () => {
    renderPanel({ explanation: 'Prefer **do-support** here.' })

    expect(screen.getByText('do-support').tagName).toBe('STRONG')
    expect(screen.queryByRole('button', { name: 'Explain' })).not.toBeInTheDocument()
  })

  it('shows an error with a retry that re-requests the explanation', () => {
    const props = renderPanel({ explainError: 'Failed to load explanation' })

    expect(screen.getByText('Failed to load explanation')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    expect(props.onExplain).toHaveBeenCalledTimes(1)
  })

  it('does not render links or images from the explanation, even when the LLM output contains them', () => {
    renderPanel({
      explanation:
        'Prefer do-support. [click here](https://evil.example/phish) ![](https://evil.example/pixel.gif)',
    })

    const explanationText = screen.getByText(/Prefer do-support\./)
    const panel = explanationText.closest('.rounded-lg') as HTMLElement
    expect(within(panel).queryByRole('link')).not.toBeInTheDocument()
    expect(within(panel).queryByRole('img')).not.toBeInTheDocument()
  })
})
