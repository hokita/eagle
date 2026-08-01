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
}
