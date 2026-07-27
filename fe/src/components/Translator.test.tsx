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
  level: 2,
  correct_count: 0,
  incorrect_count: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  mockApi.getRandomSentence.mockResolvedValue(fakeSentence)
  localStorage.clear()
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

describe('Level toggles', () => {
  it('defaults to all levels selected and fetches with no level filter on mount', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    for (const n of [1, 2, 3, 4, 5]) {
      expect(screen.getByRole('button', { name: `Level ${n}` })).toHaveAttribute('aria-pressed', 'true')
    }
    expect(mockApi.getRandomSentence).toHaveBeenCalledWith(undefined)
  })

  it('narrows the filter and persists the selection when a level is toggled off', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    const otherSentence = { ...fakeSentence, id: 2, japanese: '違う文です。', level: 3 }
    mockApi.getRandomSentence.mockResolvedValueOnce(otherSentence)
    fireEvent.click(screen.getByRole('button', { name: 'Level 1' }))
    await screen.findByText('違う文です。')
    expect(mockApi.getRandomSentence).toHaveBeenLastCalledWith([2, 3, 4, 5])
    expect(localStorage.getItem('eagle:selectedLevels')).toBe(JSON.stringify([2, 3, 4, 5]))
  })

  it('treats deselecting every level the same as selecting them all (any level)', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    for (const n of [1, 2, 3, 4, 5]) {
      mockApi.getRandomSentence.mockResolvedValueOnce(fakeSentence)
      fireEvent.click(screen.getByRole('button', { name: `Level ${n}` }))
      await screen.findByText(fakeSentence.japanese)
    }
    expect(mockApi.getRandomSentence).toHaveBeenLastCalledWith(undefined)
  })

  it('restores a previously persisted selection on mount', async () => {
    localStorage.setItem('eagle:selectedLevels', JSON.stringify([2, 4]))
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    expect(screen.getByRole('button', { name: 'Level 2' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Level 1' })).toHaveAttribute('aria-pressed', 'false')
    expect(mockApi.getRandomSentence).toHaveBeenCalledWith([2, 4])
  })

  it('resets in-progress answer state when a level is toggled', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await answerIncorrectly()
    mockApi.getRandomSentence.mockResolvedValueOnce({ ...fakeSentence, id: 3, level: 1 })
    fireEvent.click(screen.getByRole('button', { name: 'Level 1' }))
    await screen.findByLabelText(/your english translation/i)
    expect(screen.queryByText(/not quite right/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/your english translation/i)).toHaveValue('')
  })

  it('stays visible and interactive when the narrowed selection has no candidates', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    mockApi.getRandomSentence.mockRejectedValueOnce(new Error('API error: 404'))
    fireEvent.click(screen.getByRole('button', { name: 'Level 1' }))
    await screen.findByText(/api error: 404/i)
    expect(screen.getByRole('button', { name: 'Level 1' })).toBeEnabled()
  })

  it('ignores a stale response that resolves after a newer request', async () => {
    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)

    let resolveFirst: (value: typeof fakeSentence) => void = () => {}
    let resolveSecond: (value: typeof fakeSentence) => void = () => {}
    const staleSentence = { ...fakeSentence, id: 10, japanese: 'stale response' }
    const latestSentence = { ...fakeSentence, id: 20, japanese: 'latest response' }

    mockApi.getRandomSentence.mockReturnValueOnce(
      new Promise(resolve => {
        resolveFirst = resolve
      })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Level 1' }))

    mockApi.getRandomSentence.mockReturnValueOnce(
      new Promise(resolve => {
        resolveSecond = resolve
      })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Level 2' }))

    // Resolve out of arrival order: the newer (second) request settles first,
    // then the older (first) request's stale response arrives afterward.
    await act(async () => resolveSecond(latestSentence))
    await screen.findByText('latest response')

    await act(async () => resolveFirst(staleSentence))

    expect(screen.queryByText('stale response')).not.toBeInTheDocument()
    expect(screen.getByText('latest response')).toBeInTheDocument()
  })
})

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
    expect(mockApi.explainAnswer).toHaveBeenCalledWith(fakeSentence.id, 'I have no time.', 'en')
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

  it('shows an EN/JA language toggle next to the Explain button', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await answerIncorrectly()
    expect(screen.getByRole('button', { name: 'EN' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'JA' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'EN' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('does not call api.explainAnswer when switching language before explaining', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: 'JA' }))
    expect(mockApi.explainAnswer).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'JA' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('re-fetches the explanation in the new language when the toggle is switched after explaining', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    mockApi.explainAnswer.mockResolvedValueOnce({ explanation: 'English explanation.' })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    await screen.findByText('English explanation.')

    mockApi.explainAnswer.mockResolvedValueOnce({ explanation: '日本語の説明。' })
    fireEvent.click(screen.getByRole('button', { name: 'JA' }))
    await screen.findByText('日本語の説明。')

    expect(mockApi.explainAnswer).toHaveBeenLastCalledWith(fakeSentence.id, 'I have no time.', 'ja')
  })

  it('persists the selected language to localStorage and restores it on remount', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    const { unmount } = render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    fireEvent.change(screen.getByLabelText(/your english translation/i), {
      target: { value: 'I have no time.' },
    })
    fireEvent.click(screen.getByRole('button', { name: /check translation/i }))
    await screen.findByText(/not quite right/i)
    fireEvent.click(screen.getByRole('button', { name: 'JA' }))
    expect(localStorage.getItem('eagle:explainLanguage')).toBe('ja')
    unmount()

    render(<Translator user={fakeUser} />)
    await screen.findByText(fakeSentence.japanese)
    fireEvent.change(screen.getByLabelText(/your english translation/i), {
      target: { value: 'I have no time.' },
    })
    fireEvent.click(screen.getByRole('button', { name: /check translation/i }))
    await screen.findByText(/not quite right/i)
    expect(screen.getByRole('button', { name: 'JA' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('clears the previous explanation if a language-switch re-fetch fails', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    mockApi.explainAnswer.mockResolvedValueOnce({ explanation: 'English explanation.' })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: /^explain$/i }))
    await screen.findByText('English explanation.')

    mockApi.explainAnswer.mockRejectedValueOnce(new Error('API error: 500'))
    fireEvent.click(screen.getByRole('button', { name: 'JA' }))
    await screen.findByText('API error: 500')

    expect(screen.queryByText('English explanation.')).not.toBeInTheDocument()
  })

  it('disables the language toggle buttons while an explanation fetch is in progress', async () => {
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
    expect(screen.getByRole('button', { name: 'EN' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'JA' })).toBeDisabled()
    await act(async () => resolveExplain({ explanation: 'Explanation text.' }))
    expect(screen.getByRole('button', { name: 'EN' })).not.toBeDisabled()
    expect(screen.getByRole('button', { name: 'JA' })).not.toBeDisabled()
  })
})
