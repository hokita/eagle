import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('@/lib/api', () => ({ api: { savePhrase: vi.fn() } }))

import { api } from '@/lib/api'
import SavePhraseForm from './SavePhraseForm'

const mockSave = api.savePhrase as unknown as ReturnType<typeof vi.fn>

function savedPhrase(phrase: string, meaning: string, id = 'p1') {
  return {
    id,
    phrase,
    meaning,
    examples: meaning ? [`Keep it ${phrase}.`] : [],
    created_at: '2026-11-01T00:00:00Z',
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('SavePhraseForm', () => {
  it('has only a phrase box, no note', () => {
    render(<SavePhraseForm sentenceId={1} />)
    expect(screen.getByLabelText('Phrase')).toBeInTheDocument()
    expect(screen.queryByLabelText(/note/i)).not.toBeInTheDocument()
  })

  it('disables Save Phrase until a phrase is typed', () => {
    render(<SavePhraseForm sentenceId={1} />)
    const button = screen.getByRole('button', { name: 'Save Phrase' })
    expect(button).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: '   ' } })
    expect(button).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: 'to a minimum' } })
    expect(button).toBeEnabled()
  })

  it('saves the trimmed phrase, lists it with the AI meaning, and clears the box', async () => {
    mockSave.mockResolvedValue(savedPhrase('to a minimum', 'as little as possible'))
    render(<SavePhraseForm sentenceId={7} />)

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: '  to a minimum ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))

    expect(mockSave).toHaveBeenCalledWith(7, 'to a minimum')
    const list = await screen.findByRole('list', { name: 'Saved phrases' })
    expect(list).toHaveTextContent('to a minimum')
    expect(list).toHaveTextContent('as little as possible')
    expect(screen.getByLabelText('Phrase')).toHaveValue('')
  })

  it('says so when the AI could not write the meaning', async () => {
    mockSave.mockResolvedValue(savedPhrase('to a minimum', ''))
    render(<SavePhraseForm sentenceId={1} />)

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: 'to a minimum' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))

    expect(await screen.findByText(/try again on the Phrases page/)).toBeInTheDocument()
  })

  it('keeps every phrase saved from the same sentence', async () => {
    mockSave
      .mockResolvedValueOnce(savedPhrase('in the worst case', 'if the worst happens', 'p1'))
      .mockResolvedValueOnce(savedPhrase('to a minimum', 'as little as possible', 'p2'))
    render(<SavePhraseForm sentenceId={1} />)

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: 'in the worst case' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))
    await screen.findByText('in the worst case')

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: 'to a minimum' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))
    await screen.findByText('to a minimum')

    expect(screen.getAllByRole('listitem')).toHaveLength(2)
  })

  it('shows the error and keeps the draft when saving fails', async () => {
    mockSave.mockRejectedValue(new Error('API error: 500'))
    render(<SavePhraseForm sentenceId={1} />)

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: 'to a minimum' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))

    expect(await screen.findByText('API error: 500')).toBeInTheDocument()
    expect(screen.getByLabelText('Phrase')).toHaveValue('to a minimum')
    expect(screen.queryByRole('list', { name: 'Saved phrases' })).not.toBeInTheDocument()
  })

  it('disables the form while saving and the AI writes', async () => {
    let resolve: (v: unknown) => void = () => {}
    mockSave.mockReturnValue(new Promise(r => (resolve = r)))
    render(<SavePhraseForm sentenceId={1} />)

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: 'to a minimum' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))

    expect(screen.getByRole('button', { name: 'Saving… writing meaning' })).toBeDisabled()
    expect(screen.getByLabelText('Phrase')).toBeDisabled()

    resolve(savedPhrase('to a minimum', 'as little as possible'))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save Phrase' })).toBeInTheDocument(),
    )
  })
})
