import { render, screen, fireEvent, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))

vi.mock('@/lib/api', () => ({
  api: {
    getRandomSentence: vi.fn(),
    checkAnswer: vi.fn(),
    explainAnswer: vi.fn(),
    reportSentence: vi.fn(),
  },
}))

import { api } from '@/lib/api'
import Translator from './Translator'

const mockApi = api as unknown as {
  getRandomSentence: ReturnType<typeof vi.fn>
  checkAnswer: ReturnType<typeof vi.fn>
  explainAnswer: ReturnType<typeof vi.fn>
  reportSentence: ReturnType<typeof vi.fn>
}

const fakeUser = { uid: 'u1', displayName: 'Jane' } as User

const fakeSentence = {
  id: 1,
  japanese: '時間がありません。',
  english: "I don't have time.",
  page: '12',
  correct_count: 0,
  incorrect_count: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  mockApi.getRandomSentence.mockResolvedValue(fakeSentence)
})

async function answerIncorrectly() {
  render(<Translator user={fakeUser} />)
  await screen.findByText(fakeSentence.japanese)
  fireEvent.change(screen.getByLabelText(/your english translation/i), {
    target: { value: 'I have no time.' },
  })
  fireEvent.click(screen.getByRole('button', { name: /check translation/i }))
  await screen.findByText(/not quite right/i)
}

describe('Explain button', () => {
  it('is not shown before the answer is checked', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    expect(screen.queryByRole('button', { name: /^explain$/i })).not.toBeInTheDocument()
  })

  it('is not shown when the answer was correct', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: true,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    fireEvent.change(screen.getByLabelText(/your english translation/i), {
      target: { value: fakeSentence.english },
    })
    fireEvent.click(screen.getByRole('button', { name: /check translation/i }))
    await screen.findByText(/correct! well done/i)
    expect(screen.queryByRole('button', { name: /^explain$/i })).not.toBeInTheDocument()
  })

  it('is shown when the answer was incorrect', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await answerIncorrectly()
    expect(screen.getByRole('button', { name: /^explain$/i })).toBeInTheDocument()
  })

  it('calls api.explainAnswer with the sentence id and user answer', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    mockApi.explainAnswer.mockResolvedValue({ explanation: 'Great nuance explanation.' })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    await screen.findByText('Great nuance explanation.')
    expect(mockApi.explainAnswer).toHaveBeenCalledWith(fakeSentence.id, 'I have no time.')
  })

  it('shows a loading state while waiting for the explanation', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    let resolveExplain: (value: { explanation: string }) => void = () => {}
    mockApi.explainAnswer.mockReturnValue(
      new Promise(resolve => {
        resolveExplain = resolve
      })
    )
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    await screen.findByRole('button', { name: /explaining/i })
    await act(async () => resolveExplain({ explanation: 'done' }))
    await screen.findByText('done')
  })

  it('renders the explanation once it resolves', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    mockApi.explainAnswer.mockResolvedValue({
      explanation: 'Your answer is also natural; the reference is just more formal.',
    })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    expect(
      await screen.findByText('Your answer is also natural; the reference is just more formal.')
    ).toBeInTheDocument()
  })

  it('shows an error and keeps the button clickable when the call fails', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    mockApi.explainAnswer.mockRejectedValue(new Error('API error: 500'))
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    await screen.findByText('API error: 500')
    expect(screen.getByRole('button', { name: /^explain$/i })).toBeEnabled()
  })
})
