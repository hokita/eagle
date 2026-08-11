import { render, screen, fireEvent, waitFor } from '@testing-library/react'
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
    expect(screen.getByText('Level: 1, 2, 3, 4')).toBeInTheDocument()
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
