import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/api', () => ({
  api: {
    listMistakes: vi.fn(),
    getMistakesInsight: vi.fn(),
  },
}))
vi.mock('@/lib/firebase', () => ({ auth: { currentUser: { uid: 'test-uid' } } }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))

import { api } from '@/lib/api'
import Mistakes from './Mistakes'

const mockApi = api as unknown as {
  listMistakes: ReturnType<typeof vi.fn>
  getMistakesInsight: ReturnType<typeof vi.fn>
}

const fakeUser = { uid: 'test-uid', displayName: 'Jane' } as User

const oneMistake = {
  mistakes: [
    {
      sentence_id: 1,
      japanese: '時間がありません。',
      correct_answer: "I don't have time.",
      wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' }],
    },
  ],
}

async function renderWithMistakes() {
  mockApi.listMistakes.mockResolvedValue(oneMistake)
  mockApi.getMistakesInsight.mockResolvedValue({ insight: 'You drop do-support.' })
  render(<Mistakes user={fakeUser} />)
  await screen.findByText('時間がありません。')
  return await screen.findByText('You drop do-support.')
}

beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  localStorage.clear()
  mockApi.getMistakesInsight.mockResolvedValue({ insight: '' })
})

describe('Mistakes', () => {
  it('shows a loading state before the list resolves', () => {
    mockApi.listMistakes.mockReturnValue(new Promise(() => {}))
    render(<Mistakes user={fakeUser} />)
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
    render(<Mistakes user={fakeUser} />)
    await screen.findByText('時間がありません。')
    expect(screen.getByText("I don't have time.")).toBeInTheDocument()
    expect(screen.getByText('I have no time.')).toBeInTheDocument()
    expect(screen.getByText('There is no time.')).toBeInTheDocument()
  })

  it('shows an empty state when there are no mistakes', async () => {
    mockApi.listMistakes.mockResolvedValue({ mistakes: [] })
    render(<Mistakes user={fakeUser} />)
    await screen.findByText(/no mistakes yet/i)
  })

  it('shows an error state with a working retry button', async () => {
    mockApi.listMistakes.mockRejectedValueOnce(new Error('boom'))
    render(<Mistakes user={fakeUser} />)
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
    render(<Mistakes user={fakeUser} />)
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
    render(<Mistakes user={fakeUser} />)
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
    render(<Mistakes user={fakeUser} />)
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
    render(<Mistakes user={fakeUser} />)
    const insightText = await screen.findByText(/You often drop articles\./)
    const card = insightText.closest('.rounded-lg') as HTMLElement
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
    render(<Mistakes user={fakeUser} />)
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

    const { unmount } = render(<Mistakes user={fakeUser} />)
    await screen.findByText('You often drop articles.')
    expect(mockApi.getMistakesInsight).toHaveBeenCalledTimes(1)
    unmount()

    render(<Mistakes user={fakeUser} />)
    await screen.findByText('You often drop articles.')
    expect(mockApi.getMistakesInsight).toHaveBeenCalledTimes(1)
  })

  it('does not reuse a cached insight when the mistake history has changed since it was cached', async () => {
    mockApi.listMistakes
      .mockResolvedValueOnce({
        mistakes: [
          {
            sentence_id: 1,
            japanese: '時間がありません。',
            correct_answer: "I don't have time.",
            wrong_answers: [{ id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-01T00:00:00Z' }],
          },
        ],
      })
      .mockResolvedValueOnce({
        mistakes: [
          {
            sentence_id: 1,
            japanese: '時間がありません。',
            correct_answer: "I don't have time.",
            wrong_answers: [
              { id: 2, incorrect_answer: 'There is no time.', created_at: '2026-01-02T00:00:00Z' },
              { id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-01T00:00:00Z' },
            ],
          },
        ],
      })
    mockApi.getMistakesInsight
      .mockResolvedValueOnce({ insight: 'First insight.' })
      .mockResolvedValueOnce({ insight: 'Updated insight after a new mistake.' })

    const { unmount } = render(<Mistakes user={fakeUser} />)
    await screen.findByText('First insight.')
    expect(mockApi.getMistakesInsight).toHaveBeenCalledTimes(1)
    unmount()

    // Simulates returning to /mistakes after recording a new wrong answer
    // (a new history entry, id 2, sorted first) since the insight was cached.
    render(<Mistakes user={fakeUser} />)
    await screen.findByText('Updated insight after a new mistake.')
    expect(mockApi.getMistakesInsight).toHaveBeenCalledTimes(2)
  })

  it('does not fetch an insight when there are no mistakes', async () => {
    mockApi.listMistakes.mockResolvedValue({ mistakes: [] })
    render(<Mistakes user={fakeUser} />)
    await screen.findByText(/no mistakes yet/i)
    expect(mockApi.getMistakesInsight).not.toHaveBeenCalled()
  })
})

describe('header and settings', () => {
  it('renders the shared header without a link back to itself', async () => {
    await renderWithMistakes()

    expect(screen.getByRole('link', { name: 'Eagle' })).toHaveAttribute('href', '/')
    expect(screen.queryByRole('link', { name: 'Mistakes' })).not.toBeInTheDocument()
  })

  it('has no inline language toggle inside the insight card', async () => {
    await renderWithMistakes()

    expect(screen.queryByRole('tab', { name: 'EN' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'EN' })).not.toBeInTheDocument()
  })

  it('reloads the insight in the new language when settings change it', async () => {
    await renderWithMistakes()
    await screen.findByText('You drop do-support.')

    mockApi.getMistakesInsight.mockResolvedValue({ insight: '弱点の説明' })
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(await screen.findByText('弱点の説明')).toBeInTheDocument()
    expect(localStorage.getItem('eagle:explainLanguage')).toBe('ja')
  })

  it('uses the stored language for the first insight request', async () => {
    localStorage.setItem('eagle:explainLanguage', 'ja')
    await renderWithMistakes()

    await waitFor(() => expect(mockApi.getMistakesInsight).toHaveBeenCalledWith('ja'))
  })
})
