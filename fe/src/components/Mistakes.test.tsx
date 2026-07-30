import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/api', () => ({
  api: {
    listMistakes: vi.fn(),
  },
}))

import { api } from '@/lib/api'
import Mistakes from './Mistakes'

const mockApi = api as unknown as {
  listMistakes: ReturnType<typeof vi.fn>
}

beforeEach(() => {
  vi.clearAllMocks()
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
})
