import { render, screen, fireEvent, within } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ReviewPanel from './ReviewPanel'
import { answerRow, answerRows, highlightedWords } from '@/test-helpers'

const histories = [
  { id: 1, incorrect_answer: 'There is no time.', created_at: '2026-01-01T00:00:00Z' },
  { id: 2, incorrect_answer: 'I have not time.', created_at: '2026-01-02T00:00:00Z' },
]

function panelProps(overrides = {}) {
  return {
    feedback: 'incorrect' as const,
    userAnswer: 'I have no time.',
    correctAnswer: "I don't have time.",
    histories: [],
    explanation: null,
    explaining: false,
    explainError: null,
    onExplain: vi.fn(),
    followUps: [],
    asking: false,
    askError: null,
    onAsk: vi.fn(),
    ...overrides,
  }
}

function renderPanel(overrides = {}) {
  const props = panelProps(overrides)
  render(<ReviewPanel {...props} />)
  return props
}

// Some of the follow-up behaviour is about what survives a re-render — the
// question box is cleared by an answer arriving, not by the click that asked
// for it — so those tests drive the panel through new props.
function renderPanelForRerender(overrides = {}) {
  const props = panelProps(overrides)
  const view = render(<ReviewPanel {...props} />)
  return {
    props,
    rerender: (next = {}) => view.rerender(<ReviewPanel {...props} {...next} />),
  }
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

describe('follow-up questions', () => {
  const QUESTION_BOX = 'Your question about this explanation'

  function askQuestion(text: string) {
    fireEvent.change(screen.getByLabelText(QUESTION_BOX), { target: { value: text } })
    fireEvent.click(screen.getByRole('button', { name: 'Ask' }))
  }

  it('offers the question box once the explanation is on screen', () => {
    renderPanel({ explanation: 'Prefer do-support here.' })

    expect(screen.getByLabelText(QUESTION_BOX)).toBeInTheDocument()
  })

  it('has nothing to ask about before the explanation arrives', () => {
    renderPanel()

    expect(screen.queryByLabelText(QUESTION_BOX)).not.toBeInTheDocument()
  })

  it('asks the typed question', () => {
    const props = renderPanel({ explanation: 'Prefer do-support here.' })

    askQuestion('  Why is that more natural?  ')

    expect(props.onAsk).toHaveBeenCalledWith('Why is that more natural?')
  })

  it('asks on Ctrl+Enter', () => {
    const props = renderPanel({ explanation: 'Prefer do-support here.' })
    const box = screen.getByLabelText(QUESTION_BOX)

    fireEvent.change(box, { target: { value: 'Why?' } })
    fireEvent.keyDown(box, { key: 'Enter', ctrlKey: true })

    expect(props.onAsk).toHaveBeenCalledWith('Why?')
  })

  it('keeps an empty question from being asked', () => {
    const props = renderPanel({ explanation: 'Prefer do-support here.' })

    fireEvent.change(screen.getByLabelText(QUESTION_BOX), { target: { value: '   ' } })

    expect(screen.getByRole('button', { name: 'Ask' })).toBeDisabled()
    expect(props.onAsk).not.toHaveBeenCalled()
  })

  it('shows every question asked and the answer to it', () => {
    renderPanel({
      explanation: 'Prefer do-support here.',
      followUps: [
        { question: 'Why?', answer: 'Because English **negates the verb**.' },
        { question: 'Always?', answer: 'Not always.' },
      ],
    })

    expect(screen.getByText('Why?')).toBeInTheDocument()
    expect(screen.getByText('negates the verb').tagName).toBe('STRONG')
    expect(screen.getByText('Always?')).toBeInTheDocument()
    expect(screen.getByText('Not always.')).toBeInTheDocument()
  })

  it('shows a loading state while the answer is on its way', () => {
    renderPanel({ explanation: 'Prefer do-support here.', asking: true })

    expect(screen.getByText('Answering...')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Asking...' })).toBeDisabled()
    expect(screen.getByLabelText(QUESTION_BOX)).toBeDisabled()
  })

  it('clears the box once the answer arrives, ready for the next question', () => {
    const { rerender } = renderPanelForRerender({ explanation: 'Prefer do-support here.' })

    fireEvent.change(screen.getByLabelText(QUESTION_BOX), { target: { value: 'Why?' } })
    rerender({ followUps: [{ question: 'Why?', answer: 'Because.' }] })

    expect(screen.getByLabelText(QUESTION_BOX)).toHaveValue('')
  })

  it('keeps the question in the box when the answer fails, so it can be asked again', () => {
    const { rerender } = renderPanelForRerender({ explanation: 'Prefer do-support here.' })

    fireEvent.change(screen.getByLabelText(QUESTION_BOX), { target: { value: 'Why?' } })
    rerender({ askError: 'Failed to answer the question' })

    expect(screen.getByText('Failed to answer the question')).toBeInTheDocument()
    expect(screen.getByLabelText(QUESTION_BOX)).toHaveValue('Why?')
    expect(screen.getByRole('button', { name: 'Ask' })).toBeEnabled()
  })

  it('does not render links or images from an answer, even when the LLM output contains them', () => {
    renderPanel({
      explanation: 'Prefer do-support here.',
      followUps: [
        {
          question: 'Why?',
          answer: 'Because. [click here](https://evil.example/phish) ![](https://evil.example/pixel.gif)',
        },
      ],
    })

    const answer = screen.getByText(/^Because\./)
    const panel = answer.closest('.rounded-lg') as HTMLElement
    expect(within(panel).queryByRole('link')).not.toBeInTheDocument()
    expect(within(panel).queryByRole('img')).not.toBeInTheDocument()
  })
})
