import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/api', () => ({
  api: {
    getDiscussionQuestion: vi.fn(),
    discussionReply: vi.fn(),
    discussionComplete: vi.fn(),
  },
}))
vi.mock('@/lib/firebase', () => ({ auth: { currentUser: null } }))

import { api } from '@/lib/api'
import DiscussionSession from './DiscussionSession'

const user = { displayName: 'Tester', email: 't@example.com', photoURL: null } as unknown as User

const question = {
  id: 16,
  question_en: 'Who should take more responsibility for environmental problems?',
  topic: 'environment',
  level: 3,
  target_skills: ['giving opinions'],
}

const summary = {
  session_id: 's1',
  natural_english: 'I think companies are responsible, because they pollute more than anyone else.',
  naturalness_why_en: 'You opened every turn with "I think that".',
  naturalness_fix_en: 'Vary how you start a turn.',
  phrases: [
    {
      phrase: 'take responsibility for',
      meaning_en: 'to accept that something is your job',
      example_en: 'They take responsibility for it.',
    },
  ],
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.getDiscussionQuestion).mockResolvedValue(question)
})

async function startSession() {
  render(<DiscussionSession user={user} />)
  await waitFor(() =>
    expect(screen.getByText(question.question_en)).toBeInTheDocument()
  )
}

async function answerOnce(text: string) {
  fireEvent.change(screen.getByLabelText('Your answer'), { target: { value: text } })
  fireEvent.click(screen.getByRole('button', { name: 'Send' }))
}

describe('DiscussionSession', () => {
  it('loads a question and shows the conversation phase', async () => {
    await startSession()
    expect(screen.getByLabelText('Your answer')).toBeInTheDocument()
  })

  it('appends the AI follow-up after sending an answer', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: false, message: 'Why do you think so?' })
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByText('Why do you think so?')).toBeInTheDocument())
    expect(api.discussionReply).toHaveBeenCalledWith(16, [{ role: 'user', text: 'I think companies.' }])
  })

  // A done reply carries no message: the server ends the conversation once
  // both follow-ups are answered, so there is no closing line to render.
  it('moves to reflection when the server ends the conversation', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: '' })
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() =>
      expect(screen.getByText('What else did you want to say? (in Japanese)')).toBeInTheDocument()
    )
  })

  it('summarizes the session when the reflection is submitted', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: '' })
    vi.mocked(api.discussionComplete).mockResolvedValue(summary)
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByLabelText('Japanese reflection')).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Japanese reflection'), {
      target: { value: '制度を変えるべき。' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Finish' }))
    await waitFor(() => expect(screen.getByText(summary.natural_english)).toBeInTheDocument())
    expect(screen.getByText('take responsibility for')).toBeInTheDocument()
    expect(screen.getByText(summary.naturalness_why_en)).toBeInTheDocument()
    expect(screen.getByText(summary.naturalness_fix_en)).toBeInTheDocument()
    expect(api.discussionComplete).toHaveBeenCalledWith(
      16,
      [{ role: 'user', text: 'I think companies.' }],
      '制度を変えるべき。'
    )
  })

  // The summary is terminal: the only way on is a fresh question.
  it('starts a new question from the summary', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: '' })
    vi.mocked(api.discussionComplete).mockResolvedValue(summary)
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByLabelText('Japanese reflection')).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Japanese reflection'), { target: { value: 'あ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Finish' }))
    await waitFor(() => expect(screen.getByText(summary.natural_english)).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'Next question' }))
    await waitFor(() => expect(screen.getByLabelText('Your answer')).toBeInTheDocument())
    expect(screen.queryByText(summary.natural_english)).not.toBeInTheDocument()
  })

  it('shows an error with retry when the reply call fails', async () => {
    vi.mocked(api.discussionReply).mockRejectedValueOnce(new Error('API error: 500'))
    await startSession()
    await answerOnce('I think companies.')
    expect(await screen.findByText('Something went wrong. Please try again.')).toBeInTheDocument()
    vi.mocked(api.discussionReply).mockResolvedValue({ done: false, message: 'Recovered follow-up?' })
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    await waitFor(() => expect(screen.getByText('Recovered follow-up?')).toBeInTheDocument())
  })

  it('recovers via Send (not just Try Again) without creating consecutive user turns', async () => {
    vi.mocked(api.discussionReply).mockRejectedValueOnce(new Error('API error: 500'))
    await startSession()
    await answerOnce('I think companies.')
    expect(await screen.findByText('Something went wrong. Please try again.')).toBeInTheDocument()

    vi.mocked(api.discussionReply).mockResolvedValueOnce({ done: false, message: 'Recovered follow-up?' })
    await answerOnce('I think companies, actually.')

    await waitFor(() => expect(screen.getByText('Recovered follow-up?')).toBeInTheDocument())

    const lastCallTranscript = vi.mocked(api.discussionReply).mock.calls.at(-1)?.[1]
    expect(lastCallTranscript).toBeDefined()
    const roles = lastCallTranscript!.map(m => m.role)
    const lastTwo = roles.slice(-2)
    expect(lastTwo).not.toEqual(['user', 'user'])
    expect(lastCallTranscript).toEqual([{ role: 'user', text: 'I think companies, actually.' }])
  })

  it('clears the stale error banner when a failed summary is resubmitted', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: '' })
    vi.mocked(api.discussionComplete).mockRejectedValueOnce(new Error('API error: 500'))
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByLabelText('Japanese reflection')).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Japanese reflection'), { target: { value: 'あ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Finish' }))
    expect(await screen.findByText('Something went wrong. Please try again.')).toBeInTheDocument()

    vi.mocked(api.discussionComplete).mockResolvedValueOnce(summary)
    fireEvent.click(screen.getByRole('button', { name: 'Finish' }))
    await waitFor(() => expect(screen.getByText(summary.natural_english)).toBeInTheDocument())
    expect(screen.queryByText('Something went wrong. Please try again.')).not.toBeInTheDocument()
  })

  it('links to the discussion history from the page header', async () => {
    await startSession()
    expect(screen.getByRole('link', { name: 'History' })).toHaveAttribute(
      'href',
      '/discussion/history'
    )
  })

  it('shows a distinct message when the question bank is empty (404)', async () => {
    vi.mocked(api.getDiscussionQuestion).mockReset()
    vi.mocked(api.getDiscussionQuestion).mockRejectedValue(new Error('API error: 404'))
    render(<DiscussionSession user={user} />)
    expect(
      await screen.findByText('No discussion questions available yet.')
    ).toBeInTheDocument()
  })
})
