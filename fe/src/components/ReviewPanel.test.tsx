import { render, screen, fireEvent, within } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ReviewPanel from './ReviewPanel'

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
    tab: 'answer' as const,
    onTabChange: vi.fn(),
    explanation: null,
    explaining: false,
    explainError: null,
    onRetryExplain: vi.fn(),
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

describe('tabs', () => {
  it('renders no tab control at all when correct with no history', () => {
    renderPanel({ feedback: 'correct', histories: [] })

    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(screen.getByText("I don't have time.")).toBeInTheDocument()
  })

  it('adds Explain when the answer was incorrect', () => {
    renderPanel({ feedback: 'incorrect', histories: [] })

    expect(screen.getByRole('tab', { name: 'Explain' })).toBeInTheDocument()
  })

  it('adds Attempts with a count when there is history', () => {
    renderPanel({ histories })

    expect(screen.getByRole('tab', { name: 'Attempts 2' })).toBeInTheDocument()
  })

  it('omits Attempts when there is no history', () => {
    renderPanel({ histories: [] })

    expect(screen.queryByRole('tab', { name: /^Attempts/ })).not.toBeInTheDocument()
  })

  it('reports the selected tab', () => {
    const props = renderPanel({ feedback: 'incorrect' })

    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))

    expect(props.onTabChange).toHaveBeenCalledWith('explain')
  })
})

describe('answer tab', () => {
  it('shows the correct answer', () => {
    renderPanel({ tab: 'answer' })
    expect(screen.getByText("I don't have time.")).toBeInTheDocument()
  })

  it('shows what the user wrote when they were wrong', () => {
    renderPanel({ tab: 'answer', feedback: 'incorrect' })
    expect(screen.getByText('I have no time.')).toBeInTheDocument()
  })

  it('does not repeat the user answer when they were right', () => {
    renderPanel({ tab: 'answer', feedback: 'correct', userAnswer: "I don't have time." })
    expect(screen.queryByText('You wrote')).not.toBeInTheDocument()
  })
})

describe('attempts tab', () => {
  it('lists every previous incorrect answer', () => {
    renderPanel({ tab: 'attempts', histories })

    expect(screen.getByText('There is no time.')).toBeInTheDocument()
    expect(screen.getByText('I have not time.')).toBeInTheDocument()
  })
})

describe('explain tab', () => {
  it('shows a loading state while fetching', () => {
    renderPanel({ tab: 'explain', explaining: true })
    expect(screen.getByText('Explaining...')).toBeInTheDocument()
  })

  it('renders markdown as real bold text', () => {
    renderPanel({ tab: 'explain', explanation: 'Prefer **do-support** here.' })

    expect(screen.getByText('do-support').tagName).toBe('STRONG')
  })

  it('shows an error with a retry that re-requests the explanation', () => {
    const props = renderPanel({ tab: 'explain', explainError: 'Failed to load explanation' })

    expect(screen.getByText('Failed to load explanation')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    expect(props.onRetryExplain).toHaveBeenCalledTimes(1)
  })

  it('does not render links or images from the explanation, even when the LLM output contains them', () => {
    renderPanel({
      tab: 'explain',
      explanation:
        'Prefer do-support. [click here](https://evil.example/phish) ![](https://evil.example/pixel.gif)',
    })

    const explanationText = screen.getByText(/Prefer do-support\./)
    const panel = explanationText.closest('.rounded-lg') as HTMLElement
    expect(within(panel).queryByRole('link')).not.toBeInTheDocument()
    expect(within(panel).queryByRole('img')).not.toBeInTheDocument()
  })
})
