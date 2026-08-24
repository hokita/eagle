import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('./firebase', () => ({
  auth: {
    currentUser: {
      getIdToken: vi.fn().mockResolvedValue('test-token'),
    },
  },
}))

import { api } from './api'

const mockFetch = vi.fn()
global.fetch = mockFetch

beforeEach(() => {
  vi.clearAllMocks()
})

function mockResponse(body: unknown, status = 200) {
  mockFetch.mockResolvedValue({
    ok: status < 400,
    status,
    json: () => Promise.resolve(body),
  })
}

describe('api.getRandomSentence', () => {
  it('sends GET /api/sentence/random with the Authorization header', async () => {
    mockResponse({
      id: 1,
      japanese: '時間がありません。',
      english: "I don't have time.",
      page: '12',
      level: 2,
      correct_count: 0,
      incorrect_count: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    })
    const result = await api.getRandomSentence()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/sentence/random'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.id).toBe(1)
    expect(result.english).toBe("I don't have time.")
  })

  it('omits the levels query param when no levels are given', async () => {
    mockResponse({ id: 1 })
    await api.getRandomSentence()
    const [url] = mockFetch.mock.calls[0]
    expect(url).not.toContain('levels=')
  })

  it('omits the levels query param when given an empty array', async () => {
    mockResponse({ id: 1 })
    await api.getRandomSentence([])
    const [url] = mockFetch.mock.calls[0]
    expect(url).not.toContain('levels=')
  })

  it('sends the levels query param when levels are given', async () => {
    mockResponse({ id: 1 })
    await api.getRandomSentence([1, 3])
    const [url] = mockFetch.mock.calls[0]
    expect(url).toContain('/api/sentence/random?levels=1,3')
  })
})

describe('api.checkAnswer', () => {
  it('sends POST /api/answer/check with sentence_id and user_answer', async () => {
    mockResponse({ is_correct: true, correct_answer: 'Hello', histories: [] })
    const result = await api.checkAnswer(1, 'Hello')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/answer/check'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ sentence_id: 1, user_answer: 'Hello' }),
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.is_correct).toBe(true)
  })
})

describe('api.reportSentence', () => {
  it('sends POST /api/sentence/report and resolves on a 204 with no body', async () => {
    mockFetch.mockResolvedValue({ ok: true, status: 204 })
    await expect(api.reportSentence(1)).resolves.toBeUndefined()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/sentence/report'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ sentence_id: 1 }),
      })
    )
  })
})

describe('api.explainAnswer', () => {
  it('sends POST /api/answer/explain with sentence_id, user_answer, and language', async () => {
    mockResponse({ explanation: 'Your answer is also natural.' })
    const result = await api.explainAnswer(1, 'I have no time.', 'en')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/answer/explain'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          sentence_id: 1,
          user_answer: 'I have no time.',
          language: 'en',
        }),
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.explanation).toBe('Your answer is also natural.')
  })
})

describe('api.listMistakes', () => {
  it('sends GET /api/mistakes with the Authorization header', async () => {
    mockResponse({
      mistakes: [
        {
          sentence_id: 1,
          japanese: '時間がありません。',
          correct_answer: "I don't have time.",
          wrong_answers: [
            { id: 1, incorrect_answer: 'I have no time.', created_at: '2026-01-03T00:00:00Z' },
          ],
        },
      ],
    })
    const result = await api.listMistakes()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/mistakes'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.mistakes).toHaveLength(1)
    expect(result.mistakes[0].sentence_id).toBe(1)
    expect(result.mistakes[0].wrong_answers[0].incorrect_answer).toBe('I have no time.')
  })
})

describe('api.getMistakesInsight', () => {
  it('sends GET /api/mistakes/insight with the language query param', async () => {
    mockResponse({ insight: 'You often drop articles.' })
    const result = await api.getMistakesInsight('en')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/mistakes/insight?language=en'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.insight).toBe('You often drop articles.')
  })
})

describe('api error handling', () => {
  it('throws when the response is not ok', async () => {
    mockResponse({}, 500)
    await expect(api.getRandomSentence()).rejects.toThrow('API error: 500')
  })
})

describe('api.getDiscussionQuestion', () => {
  it('sends GET /api/discussion/question', async () => {
    mockResponse({
      id: 16,
      question_en: 'Who should take more responsibility for environmental problems?',
      topic: 'environment',
      level: 3,
      target_skills: ['giving opinions'],
    })
    const result = await api.getDiscussionQuestion()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/question'),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
    expect(result.id).toBe(16)
    expect(result.topic).toBe('environment')
  })
})

describe('api.discussionReply', () => {
  it('sends POST /api/discussion/reply with question_id and transcript', async () => {
    mockResponse({ done: false, message: 'Why do you think so?' })
    const transcript = [{ role: 'user' as const, text: 'I think companies.' }]
    const result = await api.discussionReply(16, transcript)
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/reply'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ question_id: 16, transcript }),
      })
    )
    expect(result.done).toBe(false)
    expect(result.message).toBe('Why do you think so?')
  })
})

describe('api.discussionAnalyze', () => {
  it('sends POST /api/discussion/analyze with the reflection', async () => {
    mockResponse({ expressed_ideas: [], missing_ideas: [], expressions: [] })
    const transcript = [{ role: 'user' as const, text: 'I think companies.' }]
    await api.discussionAnalyze(16, transcript, '制度を変えるべき。')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/analyze'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ question_id: 16, transcript, reflection_ja: '制度を変えるべき。' }),
      })
    )
  })
})

describe('api.discussionComplete', () => {
  it('sends POST /api/discussion/complete with the whole session payload', async () => {
    mockResponse({ session_id: 's1', retry_feedback: 'Nice!' })
    const payload = {
      question_id: 16,
      transcript: [{ role: 'user' as const, text: 'I think companies.' }],
      reflection_ja: '',
      expressed_ideas: [],
      missing_ideas: [],
      expressions: [],
      corrections: [],
      retry_answer: 'Companies should take responsibility.',
    }
    const result = await api.discussionComplete(payload)
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/complete'),
      expect.objectContaining({ method: 'POST', body: JSON.stringify(payload) })
    )
    expect(result.session_id).toBe('s1')
  })
})

describe('api.listDiscussionSessions / getDiscussionSession', () => {
  it('sends GET /api/discussion/sessions', async () => {
    mockResponse({ sessions: [{ id: 's1', question_en: 'Q', topic: 'work', created_at: '2026-08-23T10:00:00Z' }] })
    const result = await api.listDiscussionSessions()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/sessions'),
      expect.anything()
    )
    expect(result.sessions).toHaveLength(1)
  })

  it('sends GET /api/discussion/sessions/{id}', async () => {
    mockResponse({ id: 's1', question_en: 'Q', retry_answer: 'better' })
    const result = await api.getDiscussionSession('s1')
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/discussion/sessions/s1'),
      expect.anything()
    )
    expect(result.id).toBe('s1')
  })
})
