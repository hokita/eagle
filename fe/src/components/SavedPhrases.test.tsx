import { render, screen, fireEvent, within } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))
vi.mock('@/lib/api', () => ({
  api: { listPhrases: vi.fn(), deletePhrase: vi.fn(), rewritePhrase: vi.fn() },
}))

import { api, type SavedPhrase } from '@/lib/api'
import SavedPhrases from './SavedPhrases'

const mockApi = api as unknown as {
  listPhrases: ReturnType<typeof vi.fn>
  deletePhrase: ReturnType<typeof vi.fn>
  rewritePhrase: ReturnType<typeof vi.fn>
}

const fakeUser = { uid: 'u1', displayName: 'Jane' } as User

const minimum: SavedPhrase = {
  id: 'p2',
  phrase: 'to a minimum',
  meaning: 'as little as possible',
  examples: [
    "Let's keep the meeting to a minimum today.",
    'I try to keep my spending to a minimum before payday.',
    'Pack light and keep your luggage to a minimum.',
  ],
  created_at: '2026-11-01T10:00:00Z',
}

const worstCase: SavedPhrase = {
  id: 'p1',
  phrase: 'in the worst case',
  meaning: '',
  examples: [],
  created_at: '2026-11-01T09:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
})

function card(phrase: string): HTMLElement {
  const heading = screen.getByRole('heading', { name: phrase })
  return heading.closest('.space-y-2') as HTMLElement
}

describe('SavedPhrases', () => {
  it('lists each phrase with its meaning and examples, in English only', async () => {
    mockApi.listPhrases.mockResolvedValue({ phrases: [minimum] })
    render(<SavedPhrases user={fakeUser} />)

    expect(await screen.findByRole('heading', { name: 'to a minimum' })).toBeInTheDocument()
    expect(screen.getByText('as little as possible')).toBeInTheDocument()
    const examples = screen.getByRole('list', { name: 'Examples for to a minimum' })
    expect(
      within(examples)
        .getAllByRole('listitem')
        .map(li => li.textContent),
    ).toEqual(minimum.examples)
    expect(screen.getByText('1 saved phrase · newest first')).toBeInTheDocument()
  })

  it('does not show a Phrases link to the page it is on', async () => {
    mockApi.listPhrases.mockResolvedValue({ phrases: [minimum] })
    render(<SavedPhrases user={fakeUser} />)
    await screen.findByRole('heading', { name: 'to a minimum' })
    expect(screen.queryByRole('link', { name: 'Phrases' })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Mistakes' })).toBeInTheDocument()
  })

  it('shows the empty state with a way back to practice', async () => {
    mockApi.listPhrases.mockResolvedValue({ phrases: [] })
    render(<SavedPhrases user={fakeUser} />)

    expect(await screen.findByText('No phrases yet')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Practice a sentence' })).toHaveAttribute('href', '/')
  })

  it('shows an error and retries', async () => {
    mockApi.listPhrases
      .mockRejectedValueOnce(new Error('API error: 500'))
      .mockResolvedValueOnce({ phrases: [minimum] })
    render(<SavedPhrases user={fakeUser} />)

    expect(await screen.findByText(/Couldn't load your phrases/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))

    expect(await screen.findByRole('heading', { name: 'to a minimum' })).toBeInTheDocument()
    expect(mockApi.listPhrases).toHaveBeenCalledTimes(2)
  })

  it('replaces the examples with a fresh set', async () => {
    mockApi.listPhrases.mockResolvedValue({ phrases: [minimum] })
    mockApi.rewritePhrase.mockResolvedValue({
      ...minimum,
      examples: ['Keep the noise to a minimum, please.'],
    })
    render(<SavedPhrases user={fakeUser} />)
    await screen.findByRole('heading', { name: 'to a minimum' })

    fireEvent.click(screen.getByRole('button', { name: 'New examples' }))

    expect(await screen.findByText('Keep the noise to a minimum, please.')).toBeInTheDocument()
    expect(screen.queryByText(minimum.examples[0])).not.toBeInTheDocument()
    expect(mockApi.rewritePhrase).toHaveBeenCalledWith('p2')
  })

  it('offers Write meaning for a phrase saved without one', async () => {
    mockApi.listPhrases.mockResolvedValue({ phrases: [worstCase] })
    mockApi.rewritePhrase.mockResolvedValue({
      ...worstCase,
      meaning: 'if the worst thing happens',
      examples: ["In the worst case, we'll take a taxi."],
    })
    render(<SavedPhrases user={fakeUser} />)
    await screen.findByText('No meaning yet.')

    fireEvent.click(screen.getByRole('button', { name: 'Write meaning' }))

    expect(await screen.findByText('if the worst thing happens')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'New examples' })).toBeInTheDocument()
  })

  it('keeps the old examples and shows an error when the AI fails', async () => {
    mockApi.listPhrases.mockResolvedValue({ phrases: [minimum] })
    mockApi.rewritePhrase.mockRejectedValue(new Error('API error: 500'))
    render(<SavedPhrases user={fakeUser} />)
    await screen.findByRole('heading', { name: 'to a minimum' })

    fireEvent.click(screen.getByRole('button', { name: 'New examples' }))

    expect(await screen.findByText(/could not write this just now/)).toBeInTheDocument()
    expect(screen.getByText(minimum.examples[0])).toBeInTheDocument()
  })

  it('asks before deleting, and removes only that phrase', async () => {
    mockApi.listPhrases.mockResolvedValue({ phrases: [minimum, worstCase] })
    mockApi.deletePhrase.mockResolvedValue(undefined)
    render(<SavedPhrases user={fakeUser} />)
    await screen.findByRole('heading', { name: 'to a minimum' })

    fireEvent.click(screen.getByRole('button', { name: 'Delete to a minimum' }))
    expect(mockApi.deletePhrase).not.toHaveBeenCalled()
    const confirm = within(card('to a minimum'))
    expect(confirm.getByText('Delete this phrase?')).toBeInTheDocument()

    fireEvent.click(confirm.getByRole('button', { name: 'Delete' }))

    await screen.findByText('1 saved phrase · newest first')
    expect(mockApi.deletePhrase).toHaveBeenCalledWith('p2')
    expect(screen.queryByRole('heading', { name: 'to a minimum' })).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'in the worst case' })).toBeInTheDocument()
  })

  it('cancels a delete', async () => {
    mockApi.listPhrases.mockResolvedValue({ phrases: [minimum] })
    render(<SavedPhrases user={fakeUser} />)
    await screen.findByRole('heading', { name: 'to a minimum' })

    fireEvent.click(screen.getByRole('button', { name: 'Delete to a minimum' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(screen.queryByText('Delete this phrase?')).not.toBeInTheDocument()
    expect(mockApi.deletePhrase).not.toHaveBeenCalled()
  })

  it('keeps the phrase and says so when a delete fails', async () => {
    mockApi.listPhrases.mockResolvedValue({ phrases: [minimum] })
    mockApi.deletePhrase.mockRejectedValue(new Error('API error: 500'))
    render(<SavedPhrases user={fakeUser} />)
    await screen.findByRole('heading', { name: 'to a minimum' })

    fireEvent.click(screen.getByRole('button', { name: 'Delete to a minimum' }))
    fireEvent.click(within(card('to a minimum')).getByRole('button', { name: 'Delete' }))

    expect(await screen.findByText('Could not delete this phrase. Try again.')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'to a minimum' })).toBeInTheDocument()
  })
})
