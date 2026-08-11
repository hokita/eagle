import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))
vi.mock('@/lib/speech', () => ({ speakJapanese: vi.fn() }))

vi.mock('@/lib/api', () => ({
  api: {
    getRandomSentence: vi.fn(),
    checkAnswer: vi.fn(),
    explainAnswer: vi.fn(),
    reportSentence: vi.fn(),
  },
}))

import { api, type AnswerHistory } from '@/lib/api'
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
})

async function renderAndLoad() {
  render(<Translator user={fakeUser} />)
  await screen.findByText(fakeSentence.japanese)
}

describe('answering phase', () => {
  it('fetches a sentence with no level filter on mount', async () => {
    await renderAndLoad()
    expect(mockApi.getRandomSentence).toHaveBeenCalledWith(undefined)
  })

  it('shows the input and no review affordances', async () => {
    await renderAndLoad()

    expect(screen.getByLabelText('Your English translation')).toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(screen.queryByText('Not quite right. Try again!')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Next Sentence' })).not.toBeInTheDocument()
  })

  it('disables Check Translation until something is typed', async () => {
    await renderAndLoad()

    expect(screen.getByRole('button', { name: 'Check Translation' })).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: 'I have no time.' },
    })

    expect(screen.getByRole('button', { name: 'Check Translation' })).toBeEnabled()
  })

  it('capitalizes the first letter on blur', async () => {
    await renderAndLoad()
    const input = screen.getByLabelText('Your English translation')

    fireEvent.change(input, { target: { value: 'i have no time.' } })
    fireEvent.blur(input)

    expect(input).toHaveValue('I have no time.')
  })

  it('submits on Ctrl+Enter', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await renderAndLoad()
    const input = screen.getByLabelText('Your English translation')

    fireEvent.change(input, { target: { value: 'I have no time.' } })
    fireEvent.keyDown(input, { key: 'Enter', ctrlKey: true })

    await waitFor(() => expect(mockApi.checkAnswer).toHaveBeenCalledWith(1, 'I have no time.'))
  })

  it('shows an error with a retry when the sentence fails to load', async () => {
    mockApi.getRandomSentence.mockRejectedValue(new Error('boom'))
    render(<Translator user={fakeUser} />)

    expect(await screen.findByText('boom')).toBeInTheDocument()

    mockApi.getRandomSentence.mockResolvedValue(fakeSentence)
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))

    expect(await screen.findByText(fakeSentence.japanese)).toBeInTheDocument()
  })

  it('ignores a stale sentence response that resolves after a newer request', async () => {
    await renderAndLoad()

    let resolveStale: (value: unknown) => void = () => {}
    mockApi.getRandomSentence.mockImplementationOnce(
      () => new Promise(resolve => { resolveStale = resolve })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 1' }))

    mockApi.getRandomSentence.mockResolvedValueOnce({ ...fakeSentence, id: 9, japanese: '新しい文' })
    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 2' }))
    await screen.findByText('新しい文')

    resolveStale({ ...fakeSentence, id: 5, japanese: '古い文' })

    await waitFor(() => expect(screen.queryByText('古い文')).not.toBeInTheDocument())
    expect(screen.getByText('新しい文')).toBeInTheDocument()
  })
})

describe('settings', () => {
  it('opens the settings sheet from the header gear', async () => {
    await renderAndLoad()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))

    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeInTheDocument()
  })

  it('shows every level checked and the summary as All by default', async () => {
    await renderAndLoad()
    expect(screen.getByText('Level: All')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    for (const n of [1, 2, 3, 4, 5]) {
      expect(screen.getByRole('checkbox', { name: `Level ${n}` })).toBeChecked()
    }
  })

  it('narrows the filter, refetches, persists and updates the summary', async () => {
    await renderAndLoad()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 5' }))

    await waitFor(() =>
      expect(mockApi.getRandomSentence).toHaveBeenLastCalledWith([1, 2, 3, 4])
    )
    expect(localStorage.getItem('eagle:selectedLevels')).toBe('[1,2,3,4]')
    // The refetch is still in flight when the call assertion above passes, so
    // wait for the card to come back rather than reading it straight away.
    expect(await screen.findByText('Level: 1, 2, 3, 4')).toBeInTheDocument()
  })

  it('restores a persisted level selection on mount', async () => {
    localStorage.setItem('eagle:selectedLevels', JSON.stringify([3]))
    await renderAndLoad()

    expect(mockApi.getRandomSentence).toHaveBeenCalledWith([3])
  })

  it('persists the AI language', async () => {
    await renderAndLoad()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(localStorage.getItem('eagle:explainLanguage')).toBe('ja')
  })

  it('resets an in-progress answer when a level is toggled', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await renderAndLoad()
    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: 'I have no time.' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check Translation' }))
    await screen.findByText('Not quite right. Try again!')

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 5' }))

    await waitFor(() =>
      expect(screen.queryByText('Not quite right. Try again!')).not.toBeInTheDocument()
    )
    expect(screen.getByLabelText('Your English translation')).toHaveValue('')
  })
})

describe('header', () => {
  it('links to the mistakes page', async () => {
    await renderAndLoad()
    expect(screen.getByRole('link', { name: 'Mistakes' })).toHaveAttribute('href', '/mistakes')
  })
})

describe('review phase', () => {
  async function answerIncorrectly(histories: AnswerHistory[] = []) {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories,
    })
    await renderAndLoad()
    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: 'I have no time.' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check Translation' }))
    await screen.findByText('Not quite right. Try again!')
  }

  it('swaps the input out for the review panel', async () => {
    await answerIncorrectly()

    expect(screen.queryByLabelText('Your English translation')).not.toBeInTheDocument()
    expect(screen.getByText(fakeSentence.english)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next Sentence' })).toBeInTheDocument()
  })

  it('increments the incorrect counter', async () => {
    await answerIncorrectly()
    expect(screen.getByText(/^Incorrect: 1$/)).toBeInTheDocument()
  })

  it('shows the correct verdict and no Explain affordance when right', async () => {
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: true,
      correct_answer: fakeSentence.english,
      histories: [],
    })
    await renderAndLoad()
    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: fakeSentence.english },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check Translation' }))

    expect(await screen.findByText('Correct! Well done!')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Explain' })).not.toBeInTheDocument()
    expect(screen.getByText(/^Correct: 1$/)).toBeInTheDocument()
  })

  it('lists previous attempts alongside the answer, with no tab to open', async () => {
    await answerIncorrectly([
      { id: 1, incorrect_answer: 'There is no time.', created_at: '2026-01-01T00:00:00Z' },
    ])

    expect(screen.getByText('There is no time.')).toBeInTheDocument()
    expect(screen.getByText(fakeSentence.english)).toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
  })

  it('fetches the explanation when Explain is pressed, and shows it beside the answer', async () => {
    mockApi.explainAnswer.mockResolvedValue({ explanation: 'Prefer **do-support**.' })
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))

    await waitFor(() =>
      expect(mockApi.explainAnswer).toHaveBeenCalledWith(1, 'I have no time.', 'en')
    )
    expect(await screen.findByText('do-support')).toBeInTheDocument()
    expect(screen.getByText(fakeSentence.english)).toBeInTheDocument()
  })

  it('fetches in the stored language', async () => {
    localStorage.setItem('eagle:explainLanguage', 'ja')
    mockApi.explainAnswer.mockResolvedValue({ explanation: '説明' })
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))

    await waitFor(() =>
      expect(mockApi.explainAnswer).toHaveBeenCalledWith(1, 'I have no time.', 'ja')
    )
  })

  it('does not fetch an explanation until Explain is pressed', async () => {
    await answerIncorrectly()

    expect(screen.getByRole('button', { name: 'Explain' })).toBeInTheDocument()
    expect(mockApi.explainAnswer).not.toHaveBeenCalled()
  })

  it('retires the Explain button once the explanation is on screen', async () => {
    mockApi.explainAnswer.mockResolvedValue({ explanation: 'Once.' })
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))
    await screen.findByText('Once.')

    expect(screen.queryByRole('button', { name: 'Explain' })).not.toBeInTheDocument()
    expect(mockApi.explainAnswer).toHaveBeenCalledTimes(1)
  })

  it('re-fetches in the new language when the setting changes after explaining', async () => {
    mockApi.explainAnswer.mockResolvedValue({ explanation: 'English text' })
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))
    await screen.findByText('English text')

    mockApi.explainAnswer.mockResolvedValue({ explanation: '日本語のテキスト' })
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(await screen.findByText('日本語のテキスト')).toBeInTheDocument()
  })

  it('refetches in the new language when the setting changes while an explanation is in flight, and never renders the superseded response', async () => {
    let resolveStale: (value: unknown) => void = () => {}
    mockApi.explainAnswer.mockImplementationOnce(
      () => new Promise(resolve => { resolveStale = resolve })
    )
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))
    await waitFor(() => expect(mockApi.explainAnswer).toHaveBeenCalledTimes(1))

    mockApi.explainAnswer.mockResolvedValueOnce({ explanation: '日本語のテキスト' })
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(await screen.findByText('日本語のテキスト')).toBeInTheDocument()
    await waitFor(() => expect(mockApi.explainAnswer).toHaveBeenCalledTimes(2))
    expect(mockApi.explainAnswer).toHaveBeenLastCalledWith(1, 'I have no time.', 'ja')

    await act(async () => {
      resolveStale({ explanation: 'Stale English explanation' })
      await Promise.resolve()
    })

    expect(screen.queryByText('Stale English explanation')).not.toBeInTheDocument()
    expect(screen.getByText('日本語のテキスト')).toBeInTheDocument()
  })

  it('does not call explainAnswer when the language changes before explaining', async () => {
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(mockApi.explainAnswer).not.toHaveBeenCalled()
  })

  it('shows an explain error with a working retry', async () => {
    mockApi.explainAnswer.mockRejectedValue(new Error('Explain failed'))
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))
    expect(await screen.findByText('Explain failed')).toBeInTheDocument()

    mockApi.explainAnswer.mockResolvedValue({ explanation: 'Recovered.' })
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))

    expect(await screen.findByText('Recovered.')).toBeInTheDocument()
  })

  it('discards an explanation superseded by moving to the next sentence', async () => {
    let resolveStale: (value: unknown) => void = () => {}
    mockApi.explainAnswer.mockImplementationOnce(
      () => new Promise(resolve => { resolveStale = resolve })
    )
    await answerIncorrectly()
    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))

    fireEvent.click(screen.getByRole('button', { name: 'Next Sentence' }))
    await screen.findByLabelText('Your English translation')

    resolveStale({ explanation: 'Stale explanation' })

    await waitFor(() =>
      expect(screen.queryByText('Stale explanation')).not.toBeInTheDocument()
    )
  })

  it('does not skip the fetch or leak a stale explanation into the next sentence\'s explanation', async () => {
    const secondSentence = {
      ...fakeSentence,
      id: 2,
      japanese: '二番目の文です。',
      english: 'This is the second sentence.',
    }

    let resolveFirstExplain: (value: unknown) => void = () => {}
    mockApi.explainAnswer.mockImplementationOnce(
      () => new Promise(resolve => { resolveFirstExplain = resolve })
    )
    mockApi.checkAnswer.mockResolvedValue({
      is_correct: false,
      correct_answer: fakeSentence.english,
      histories: [],
    })

    // Sentence 1: answer incorrectly, open Explain — the fetch is left in flight.
    await renderAndLoad()
    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: 'I have no time.' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check Translation' }))
    await screen.findByText('Not quite right. Try again!')
    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))
    await waitFor(() => expect(mockApi.explainAnswer).toHaveBeenCalledTimes(1))

    // Move to sentence 2 and answer it incorrectly too, all while sentence 1's
    // explain request is still pending.
    mockApi.getRandomSentence.mockResolvedValueOnce(secondSentence)
    fireEvent.click(screen.getByRole('button', { name: 'Next Sentence' }))
    await screen.findByText(secondSentence.japanese)

    fireEvent.change(screen.getByLabelText('Your English translation'), {
      target: { value: 'Some other answer.' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check Translation' }))
    await screen.findByText('Not quite right. Try again!')

    // Now sentence 1's stale request resolves, after we've already moved on.
    mockApi.explainAnswer.mockResolvedValue({ explanation: 'Second sentence explanation' })
    await act(async () => {
      resolveFirstExplain({ explanation: 'Stale explanation from sentence one' })
      await Promise.resolve()
    })

    // Pressing Explain on sentence 2 must still fetch — not be skipped because
    // a stale explanation value leaked into state — and must never show sentence
    // 1's text.
    fireEvent.click(screen.getByRole('button', { name: 'Explain' }))

    await waitFor(() =>
      expect(mockApi.explainAnswer).toHaveBeenCalledWith(2, 'Some other answer.', 'en')
    )
    expect(await screen.findByText('Second sentence explanation')).toBeInTheDocument()
    expect(screen.queryByText('Stale explanation from sentence one')).not.toBeInTheDocument()
  })

  it('reports the sentence and acknowledges it', async () => {
    mockApi.reportSentence.mockResolvedValue(undefined)
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Report' }))

    expect(await screen.findByRole('button', { name: 'Reported' })).toBeInTheDocument()
    expect(mockApi.reportSentence).toHaveBeenCalledWith(1)
  })

  it('resets to the answering phase on Next Sentence', async () => {
    await answerIncorrectly()

    fireEvent.click(screen.getByRole('button', { name: 'Next Sentence' }))

    expect(await screen.findByLabelText('Your English translation')).toHaveValue('')
    expect(screen.queryByText('Not quite right. Try again!')).not.toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
  })
})
