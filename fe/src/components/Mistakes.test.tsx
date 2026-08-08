import { render, screen, fireEvent, within } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/api', () => ({
  api: {
    listMistakes: vi.fn(),
    getMistakesInsight: vi.fn(),
  },
}))
vi.mock('@/lib/firebase', () => ({ auth: { currentUser: { uid: 'test-uid' } } }))

import { api } from '@/lib/api'
import Mistakes from './Mistakes'

const mockApi = api as unknown as {
  listMistakes: ReturnType<typeof vi.fn>
  getMistakesInsight: ReturnType<typeof vi.fn>
}

beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  mockApi.getMistakesInsight.mockResolvedValue({ insight: '' })
})

describe('Mistakes', () => {
  it('shows a loading state before the list resolves', () => {
    mockApi.listMistakes.mockReturnValue(new Promise(() => {}))
    render(<Mistakes />)
    expect(screen.getByText(/loading/i)).toBeInTheDocument()
  })

  it('renders each mistake with its correct answer and wrong answers', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [
            { id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' },
            { id: 2, incorrect_answer: 'There is no time.', created_at: '2026-01-01T00:00:00Z' },
          ],
        },
      ],
    })
    render(<Mistakes />)
    await screen.findByText('時間がありません。')
    expect(screen.getByText("I don't have time.")).toBeInTheDocument()
    expect(screen.getByText('I have no time.')).toBeInTheDocument()
    expect(screen.getByText('There is no time.')).toBeInTheDocument()
  })

  it('shows an empty state when there are no mistakes', async () => {
    mockApi.listMistakes.mockResolvedValue({ mistakes: [] })
    render(<Mistakes />)
    await screen.findByText(/no mistakes yet/i)
  })

  it('shows an error state with a working retry button', async () => {
    mockApi.listMistakes.mockRejectedValueOnce(new Error('boom'))
    render(<Mistakes />)
    await screen.findByText('boom')
    mockApi.listMistakes.mockResolvedValueOnce({ mistakes: [] })
    fireEvent.click(screen.getByRole('button', { name: /try again/i }))
    await screen.findByText(/no mistakes yet/i)
  })

  it('fetches and renders the weakness insight above the list', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockResolvedValue({ insight: 'You often drop articles.' })
    render(<Mistakes />)
    await screen.findByText('You often drop articles.')
    expect(screen.getByText('時間がありません。')).toBeInTheDocument()
  })

  it('shows an insight loading state while the insight is pending', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockReturnValue(new Promise(() => {}))
    render(<Mistakes />)
    await screen.findByText(/analyzing your mistakes/i)
  })

  it('renders a markdown-formatted insight as real lists and bold text', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockResolvedValue({
      insight: 'You often drop articles.\n\n- **Articles**: remember "a" and "the".',
    })
    render(<Mistakes />)
    await screen.findByText('You often drop articles.')
    expect(screen.getByRole('list')).toBeInTheDocument()
    expect(screen.getByText('Articles')).toBeInTheDocument()
    expect(screen.getByText('Articles').tagName).toBe('STRONG')
    expect(screen.queryByText(/\*\*Articles\*\*/)).not.toBeInTheDocument()
    expect(screen.queryByText(/^- /)).not.toBeInTheDocument()
  })

  it('does not render links or images from the insight, even when the LLM output contains them', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockResolvedValue({
      insight: 'You often drop articles. [click here](https://evil.example/phish) ![](https://evil.example/pixel.gif)',
    })
    render(<Mistakes />)
    const insightText = await screen.findByText(/You often drop articles\./)
    const card = insightText.closest('.border-indigo-300') as HTMLElement
    expect(within(card).queryByRole('link')).not.toBeInTheDocument()
    expect(within(card).queryByRole('img')).not.toBeInTheDocument()
  })

  it('shows an insight error with a working retry button', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockRejectedValueOnce(new Error('insight boom'))
    render(<Mistakes />)
    await screen.findByText('insight boom')
    mockApi.getMistakesInsight.mockResolvedValueOnce({ insight: 'You often drop articles.' })
    fireEvent.click(screen.getByRole('button', { name: /try again/i }))
    await screen.findByText('You often drop articles.')
  })

  it('reuses a cached insight on remount instead of calling the API again', async () => {
    mockApi.listMistakes.mockResolvedValue({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
        },
      ],
    })
    mockApi.getMistakesInsight.mockResolvedValue({ insight: 'You often drop articles.' })

    const { unmount } = render(<Mistakes />)
    await screen.findByText('You often drop articles.')
    expect(mockApi.getMistakesInsight).toHaveBeenCalledTimes(1)
    unmount()

    render(<Mistakes />)
    await screen.findByText('You often drop articles.')
    expect(mockApi.getMistakesInsight).toHaveBeenCalledTimes(1)
  })

  it('does not fetch an insight when there are no mistakes', async () => {
    mockApi.listMistakes.mockResolvedValue({ mistakes: [] })
    render(<Mistakes />)
    await screen.findByText(/no mistakes yet/i)
    expect(mockApi.getMistakesInsight).not.toHaveBeenCalled()
  })
})
