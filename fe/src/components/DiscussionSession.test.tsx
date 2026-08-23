import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/api', () => ({
  api: {
    getDiscussionQuestion: vi.fn(),
    discussionReply: vi.fn(),
    discussionAnalyze: vi.fn(),
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

const analysis = {
  expressed_ideas: ['Companies are responsible.'],
  missing_ideas: ['Systemic change is needed.'],
  expressions: [
    { phrase: 'take responsibility for', meaning_ja: '〜に責任を持つ', example_en: 'x' },
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

  it('moves to reflection when the coach says done', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: 'Thanks for sharing!' })
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() =>
      expect(screen.getByText('日本語で答えるなら、他に言いたかったことはありますか？')).toBeInTheDocument()
    )
    expect(screen.getByText('Thanks for sharing!')).toBeInTheDocument()
  })

  it('analyzes the reflection and shows the study phase', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: 'Thanks!' })
    vi.mocked(api.discussionAnalyze).mockResolvedValue(analysis)
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByLabelText('Japanese reflection')).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Japanese reflection'), {
      target: { value: '制度を変えるべき。' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))
    await waitFor(() => expect(screen.getByText('take responsibility for')).toBeInTheDocument())
  })

  it('skipping reflection goes straight to retry with no analyze call', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: 'Thanks!' })
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByLabelText('Japanese reflection')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Nothing to add — skip' }))
    expect(await screen.findByLabelText('Your improved answer')).toBeInTheDocument()
    expect(api.discussionAnalyze).not.toHaveBeenCalled()
  })

  it('completes the session and shows the comparison', async () => {
    vi.mocked(api.discussionReply).mockResolvedValue({ done: true, message: 'Thanks!' })
    vi.mocked(api.discussionAnalyze).mockResolvedValue(analysis)
    vi.mocked(api.discussionComplete).mockResolvedValue({
      session_id: 's1',
      retry_feedback: 'You used the new expression!',
    })
    await startSession()
    await answerOnce('I think companies.')
    await waitFor(() => expect(screen.getByLabelText('Japanese reflection')).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Japanese reflection'), { target: { value: 'あ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Try the question again' })).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Try the question again' }))
    fireEvent.change(screen.getByLabelText('Your improved answer'), {
      target: { value: 'Companies should take responsibility.' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Submit answer' }))
    await waitFor(() => expect(screen.getByText('You used the new expression!')).toBeInTheDocument())
    expect(api.discussionComplete).toHaveBeenCalledWith(
      expect.objectContaining({
        question_id: 16,
        retry_answer: 'Companies should take responsibility.',
        expressions: analysis.expressions,
      })
    )
    expect(screen.getByText('I think companies.')).toBeInTheDocument()
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
