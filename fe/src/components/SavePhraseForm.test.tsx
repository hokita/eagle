import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('@/lib/api', () => ({ api: { savePhrase: vi.fn() } }))

import { api } from '@/lib/api'
import SavePhraseForm from './SavePhraseForm'

const mockSave = api.savePhrase as unknown as ReturnType<typeof vi.fn>

function savedPhrase(phrase: string, note: string, id = 'p1') {
  return {
    id,
    sentence_id: 1,
    phrase,
    note,
    japanese: '時間がありません。',
    english: "I don't have time.",
    created_at: '2026-11-01T00:00:00Z',
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('SavePhraseForm', () => {
  it('disables Save Phrase until a phrase is typed', () => {
    render(<SavePhraseForm sentenceId={1} />)
    const button = screen.getByRole('button', { name: 'Save Phrase' })
    expect(button).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: '   ' } })
    expect(button).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: "don't have time" } })
    expect(button).toBeEnabled()
  })

  it('saves the trimmed phrase and note, lists it, and clears the form', async () => {
    mockSave.mockResolvedValue(savedPhrase("don't have time", '時間がない'))
    render(<SavePhraseForm sentenceId={7} />)

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: "  don't have time " } })
    fireEvent.change(screen.getByLabelText('Note (optional)'), {
      target: { value: ' 時間がない ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))

    expect(mockSave).toHaveBeenCalledWith(7, "don't have time", '時間がない')
    const list = await screen.findByRole('list', { name: 'Saved phrases' })
    expect(list).toHaveTextContent("don't have time")
    expect(list).toHaveTextContent('時間がない')
    expect(screen.getByLabelText('Phrase')).toHaveValue('')
    expect(screen.getByLabelText('Note (optional)')).toHaveValue('')
  })

  it('keeps every phrase saved from the same sentence', async () => {
    mockSave
      .mockResolvedValueOnce(savedPhrase('have time', '', 'p1'))
      .mockResolvedValueOnce(savedPhrase("don't", '', 'p2'))
    render(<SavePhraseForm sentenceId={1} />)

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: 'have time' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))
    await screen.findByText('have time')

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: "don't" } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))
    await screen.findByText("don't")

    expect(screen.getAllByRole('listitem')).toHaveLength(2)
  })

  it('shows the error and keeps the draft when saving fails', async () => {
    mockSave.mockRejectedValue(new Error('API error: 500'))
    render(<SavePhraseForm sentenceId={1} />)

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: 'have time' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))

    expect(await screen.findByText('API error: 500')).toBeInTheDocument()
    expect(screen.getByLabelText('Phrase')).toHaveValue('have time')
    expect(screen.queryByRole('list', { name: 'Saved phrases' })).not.toBeInTheDocument()
  })

  it('disables the form while saving', async () => {
    let resolve: (v: unknown) => void = () => {}
    mockSave.mockReturnValue(new Promise(r => (resolve = r)))
    render(<SavePhraseForm sentenceId={1} />)

    fireEvent.change(screen.getByLabelText('Phrase'), { target: { value: 'have time' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Phrase' }))

    expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
    expect(screen.getByLabelText('Phrase')).toBeDisabled()

    resolve(savedPhrase('have time', ''))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save Phrase' })).toBeInTheDocument(),
    )
  })
})
