import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/api', () => ({
  api: {
    listDiscussionSessions: vi.fn(),
    getDiscussionSession: vi.fn(),
  },
}))
vi.mock('@/lib/firebase', () => ({ auth: { currentUser: null } }))

import { api } from '@/lib/api'
import SessionHistory from './SessionHistory'

const user = { displayName: 'Tester', email: 't@example.com', photoURL: null } as unknown as User

const summary = {
  id: 's1',
  question_en: 'Who is responsible?',
  topic: 'environment',
  created_at: '2026-08-23T10:00:00Z',
}

const detail = {
  id: 's1',
  question_id: 16,
  question_en: 'Who is responsible?',
  topic: 'environment',
  transcript: [
    { role: 'user' as const, text: 'I think companies.' },
    { role: 'ai' as const, text: 'Why?' },
  ],
  reflection_ja: '制度を変えるべき。',
  natural_english: 'I think companies are responsible, and they should change the system.',
  naturalness_why_en: 'You opened every turn with "I think that".',
  naturalness_fix_en: 'Vary how you start a turn.',
  phrases: [
    { phrase: 'take responsibility for', meaning_en: 'to accept it is your job', example_en: 'x' },
  ],
  created_at: '2026-08-23T10:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('SessionHistory', () => {
  it('lists completed sessions', async () => {
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary] })
    render(<SessionHistory user={user} />)
    expect(await screen.findByText('Who is responsible?')).toBeInTheDocument()
    expect(screen.getByText('environment')).toBeInTheDocument()
  })

  it('shows an empty state', async () => {
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [] })
    render(<SessionHistory user={user} />)
    expect(await screen.findByText('No sessions yet — try a discussion!')).toBeInTheDocument()
  })

  it('expands a session inline with its details', async () => {
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary] })
    vi.mocked(api.getDiscussionSession).mockResolvedValue(detail)
    render(<SessionHistory user={user} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Who is responsible?' }))
    await waitFor(() => expect(screen.getByText(detail.natural_english)).toBeInTheDocument())
    expect(api.getDiscussionSession).toHaveBeenCalledWith('s1')
    expect(screen.getByText('制度を変えるべき。')).toBeInTheDocument()
    expect(screen.getByText('take responsibility for')).toBeInTheDocument()
    expect(screen.getByText(detail.naturalness_why_en)).toBeInTheDocument()
    expect(screen.getByText(detail.naturalness_fix_en)).toBeInTheDocument()
  })

  it('shows an error with retry when loading fails', async () => {
    vi.mocked(api.listDiscussionSessions).mockRejectedValueOnce(new Error('API error: 500'))
    render(<SessionHistory user={user} />)
    expect(await screen.findByText('Failed to load sessions.')).toBeInTheDocument()
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary] })
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    expect(await screen.findByText('Who is responsible?')).toBeInTheDocument()
  })

  // Sessions saved before the summary replaced the study/retry flow carry
  // neither field; the card must still open rather than crash on them.
  it('renders a legacy session that has no summary', async () => {
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary] })
    vi.mocked(api.getDiscussionSession).mockResolvedValue({
      ...detail,
      natural_english: '',
      naturalness_why_en: '',
      naturalness_fix_en: '',
      phrases: [],
    })
    render(<SessionHistory user={user} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Who is responsible?' }))
    expect(await screen.findByText('I think companies.')).toBeInTheDocument()
    expect(screen.queryByText('Natural English')).not.toBeInTheDocument()
    expect(screen.queryByText('Useful phrases')).not.toBeInTheDocument()
    expect(screen.queryByText('Why it sounded unnatural')).not.toBeInTheDocument()
  })

  it('scopes a failed detail fetch to the session card, keeping the list visible', async () => {
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary] })
    vi.mocked(api.getDiscussionSession).mockRejectedValueOnce(new Error('API error: 500'))
    render(<SessionHistory user={user} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Who is responsible?' }))
    expect(await screen.findByText('Failed to load the session.')).toBeInTheDocument()
    expect(screen.getByText('Who is responsible?')).toBeInTheDocument()

    vi.mocked(api.getDiscussionSession).mockResolvedValue(detail)
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    expect(await screen.findByText(detail.natural_english)).toBeInTheDocument()
  })

  it('a stale detail failure never surfaces in another session card', async () => {
    const summary2 = { ...summary, id: 's2', question_en: 'Second question?' }
    const detail2 = { ...detail, id: 's2', question_en: 'Second question?', natural_english: 'Second summary!' }
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary, summary2] })
    let rejectFirst: (err: Error) => void = () => {}
    vi.mocked(api.getDiscussionSession).mockImplementation(id =>
      id === 's1'
        ? new Promise((_, reject) => {
            rejectFirst = reject
          })
        : Promise.resolve(detail2)
    )
    render(<SessionHistory user={user} />)

    // Open s1 (fetch stays pending), then switch to s2, which succeeds.
    fireEvent.click(await screen.findByRole('button', { name: 'Who is responsible?' }))
    fireEvent.click(screen.getByRole('button', { name: 'Second question?' }))
    await waitFor(() => expect(screen.getByText('Second summary!')).toBeInTheDocument())

    // s1's late failure must not surface inside s2's open card.
    await act(async () => {
      rejectFirst(new Error('API error: 500'))
    })
    expect(screen.queryByText('Failed to load the session.')).not.toBeInTheDocument()
    expect(screen.getByText('Second summary!')).toBeInTheDocument()
  })

  it("a stale failure does not wipe another card's visible error", async () => {
    const summary2 = { ...summary, id: 's2', question_en: 'Second question?' }
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary, summary2] })
    let rejectFirst: (err: Error) => void = () => {}
    vi.mocked(api.getDiscussionSession).mockImplementation(id =>
      id === 's1'
        ? new Promise((_, reject) => {
            rejectFirst = reject
          })
        : Promise.reject(new Error('API error: 500'))
    )
    render(<SessionHistory user={user} />)

    // Open s1 (fetch stays pending), switch to s2, which fails immediately.
    fireEvent.click(await screen.findByRole('button', { name: 'Who is responsible?' }))
    fireEvent.click(screen.getByRole('button', { name: 'Second question?' }))
    expect(await screen.findByText('Failed to load the session.')).toBeInTheDocument()

    // s1's late failure must not clear s2's error or its retry control.
    await act(async () => {
      rejectFirst(new Error('API error: 500'))
    })
    expect(screen.getByText('Failed to load the session.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Try Again' })).toBeInTheDocument()
  })

  it('a superseded request for the same session cannot contradict the newer one', async () => {
    vi.mocked(api.listDiscussionSessions).mockResolvedValue({ sessions: [summary] })
    let rejectFirst: (err: Error) => void = () => {}
    vi.mocked(api.getDiscussionSession)
      .mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            rejectFirst = reject
          })
      )
      .mockResolvedValueOnce(detail)
    render(<SessionHistory user={user} />)
    const card = await screen.findByRole('button', { name: 'Who is responsible?' })

    // Open (first fetch stays pending), close, reopen (second fetch succeeds).
    fireEvent.click(card)
    fireEvent.click(card)
    fireEvent.click(card)
    await waitFor(() => expect(screen.getByText(detail.natural_english)).toBeInTheDocument())

    // The stale first request's late failure must not add an error next to
    // the successfully rendered detail.
    await act(async () => {
      rejectFirst(new Error('API error: 500'))
    })
    expect(screen.queryByText('Failed to load the session.')).not.toBeInTheDocument()
    expect(screen.getByText(detail.natural_english)).toBeInTheDocument()
  })
})
