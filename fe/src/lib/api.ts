import { auth } from './firebase'

export interface Sentence {
  id: number
  japanese: string
  english: string
  page: string
  level: number
  correct_count: number
  incorrect_count: number
  created_at: string
  updated_at: string
}

export interface AnswerHistory {
  id: number
  incorrect_answer: string
  created_at: string
}

export interface Mistake {
  sentence_id: number
  japanese: string
  correct_answer: string
  wrong_answers: AnswerHistory[]
}

export interface CheckAnswerResponse {
  is_correct: boolean
  correct_answer: string
  histories: AnswerHistory[]
}

export interface ExplainResponse {
  explanation: string
}

export interface MistakesInsightResponse {
  insight: string
}

export interface DiscussionQuestion {
  id: number
  question_en: string
  topic: string
  level: number
  target_skills: string[]
}

export interface DiscussionMessage {
  role: 'user' | 'ai'
  text: string
}

export interface DiscussionReplyResponse {
  done: boolean
  message: string
}

export interface Expression {
  phrase: string
  meaning_ja: string
  example_en: string
}

export interface Correction {
  original: string
  better: string
  note_ja: string
}

export interface GapAnalysis {
  expressed_ideas: string[]
  missing_ideas: string[]
  expressions: Expression[]
  corrections: Correction[]
}

export interface DiscussionCompleteRequest {
  question_id: number
  transcript: DiscussionMessage[]
  reflection_ja: string
  expressed_ideas: string[]
  missing_ideas: string[]
  expressions: Expression[]
  corrections: Correction[]
  retry_answer: string
}

export interface DiscussionCompleteResponse {
  session_id: string
  retry_feedback: string
}

export interface DiscussionSessionSummary {
  id: string
  question_en: string
  topic: string
  created_at: string
}

export interface DiscussionSessionDetail {
  id: string
  question_id: number
  question_en: string
  topic: string
  transcript: DiscussionMessage[]
  reflection_ja: string
  expressed_ideas: string[]
  missing_ideas: string[]
  expressions: Expression[]
  corrections: Correction[]
  first_answer: string
  retry_answer: string
  retry_feedback: string
  created_at: string
}

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? ''

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = await auth.currentUser?.getIdToken()
  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
      ...options?.headers,
    },
  })
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  getRandomSentence: (levels?: number[]) =>
    request<Sentence>(
      `/api/sentence/random${levels && levels.length > 0 ? `?levels=${levels.join(',')}` : ''}`
    ),

  checkAnswer: (sentenceId: number, userAnswer: string) =>
    request<CheckAnswerResponse>('/api/answer/check', {
      method: 'POST',
      body: JSON.stringify({ sentence_id: sentenceId, user_answer: userAnswer }),
    }),

  reportSentence: (sentenceId: number) =>
    request<void>('/api/sentence/report', {
      method: 'POST',
      body: JSON.stringify({ sentence_id: sentenceId }),
    }),

  listMistakes: () => request<{ mistakes: Mistake[] }>('/api/mistakes'),

  getMistakesInsight: (language: 'en' | 'ja') =>
    request<MistakesInsightResponse>(`/api/mistakes/insight?language=${language}`),

  explainAnswer: (sentenceId: number, userAnswer: string, language: 'en' | 'ja') =>
    request<ExplainResponse>('/api/answer/explain', {
      method: 'POST',
      body: JSON.stringify({ sentence_id: sentenceId, user_answer: userAnswer, language }),
    }),

  getDiscussionQuestion: () => request<DiscussionQuestion>('/api/discussion/question'),

  discussionReply: (questionId: number, transcript: DiscussionMessage[]) =>
    request<DiscussionReplyResponse>('/api/discussion/reply', {
      method: 'POST',
      body: JSON.stringify({ question_id: questionId, transcript }),
    }),

  discussionAnalyze: (questionId: number, transcript: DiscussionMessage[], reflectionJa: string) =>
    request<GapAnalysis>('/api/discussion/analyze', {
      method: 'POST',
      body: JSON.stringify({ question_id: questionId, transcript, reflection_ja: reflectionJa }),
    }),

  discussionComplete: (payload: DiscussionCompleteRequest) =>
    request<DiscussionCompleteResponse>('/api/discussion/complete', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  listDiscussionSessions: () =>
    request<{ sessions: DiscussionSessionSummary[] }>('/api/discussion/sessions'),

  getDiscussionSession: (id: string) =>
    request<DiscussionSessionDetail>(`/api/discussion/sessions/${id}`),
}
