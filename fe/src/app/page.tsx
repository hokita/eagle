'use client'

import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { CheckCircle, XCircle } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import Image from 'next/image'

interface Sentence {
  id: number
  japanese: string
  english: string
  page: string
  correct_count: number
  incorrect_count: number
  created_at: string
  updated_at: string
}

interface AnswerHistory {
  id: number
  incorrect_answer: string
  created_at: string
}

interface CheckAnswerResponse {
  is_correct: boolean
  correct_answer: string
  histories: AnswerHistory[]
}

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

export default function JapaneseTranslator() {
  const [currentSentence, setCurrentSentence] = useState<Sentence | null>(null)
  const [userTranslation, setUserTranslation] = useState('')
  const [feedback, setFeedback] = useState<'correct' | 'incorrect' | null>(null)
  const [showAnswer, setShowAnswer] = useState(false)
  const [loading, setLoading] = useState(true)
  const [histories, setHistories] = useState<AnswerHistory[]>([])
  const [error, setError] = useState<string | null>(null)
  const [correctCount, setCorrectCount] = useState(0)
  const [incorrectCount, setIncorrectCount] = useState(0)

  const getRandomSentence = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await fetch(`${API_BASE_URL}/api/sentence/random`)
      if (!response.ok) {
        throw new Error('Failed to fetch sentence')
      }
      const sentence: Sentence = await response.json()
      setCurrentSentence(sentence)
      setCorrectCount(sentence.correct_count)
      setIncorrectCount(sentence.incorrect_count)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sentence')
    } finally {
      setLoading(false)
    }
  }

  const checkTranslation = async () => {
    if (!currentSentence) return

    try {
      const response = await fetch(`${API_BASE_URL}/api/answer/check`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          sentence_id: currentSentence.id,
          user_answer: userTranslation,
        }),
      })

      if (!response.ok) {
        throw new Error('Failed to check answer')
      }

      const result: CheckAnswerResponse = await response.json()
      setFeedback(result.is_correct ? 'correct' : 'incorrect')
      setHistories(result.histories)
      setShowAnswer(true)

      // Update counters
      if (result.is_correct) {
        setCorrectCount(prev => prev + 1)
      } else {
        setIncorrectCount(prev => prev + 1)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to check answer')
    }
  }

  const nextSentence = () => {
    setUserTranslation('')
    setFeedback(null)
    setShowAnswer(false)
    setHistories([])
    setError(null)
    setCorrectCount(0)
    setIncorrectCount(0)
    getRandomSentence()
  }

  useEffect(() => {
    getRandomSentence()
  }, [])

  if (loading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading...</p>
        </div>
      </div>
    )
  }

  if (error || !currentSentence) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4 flex items-center justify-center">
        <Card className="max-w-md">
          <CardHeader>
            <CardTitle className="text-red-600">Error</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-gray-700 mb-4">{error || 'Failed to load content'}</p>
            <Button onClick={() => getRandomSentence()} className="w-full">
              Try Again
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <div className="text-center mb-8">
          <div className="flex items-center justify-center gap-2 mb-4">
            <Image src="/eagle-thumbnail.png" alt="Eagle logo" width={32} height={32} />
            <h1 className="text-3xl font-bold text-gray-900">Eagle</h1>
          </div>
        </div>

        <div className="grid gap-6 mb-6">
          <Card>
            <CardHeader>
              <CardTitle>Translate this sentence</CardTitle>
              <CardDescription>Translate the Japanese sentence below into English</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="text-center">
                <div className="text-3xl font-bold text-gray-900 mb-2">
                  {currentSentence.japanese}
                </div>
                <div className="flex justify-center gap-4 text-sm text-gray-600 mt-2">
                  <div className="flex items-center gap-1">
                    <CheckCircle className="h-4 w-4 text-green-500" />
                    <span>Correct: {correctCount}</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <XCircle className="h-4 w-4 text-red-500" />
                    <span>Incorrect: {incorrectCount}</span>
                  </div>
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="translation">Your English translation:</Label>
                <Textarea
                  id="translation"
                  value={userTranslation}
                  onChange={e => setUserTranslation(e.target.value)}
                  placeholder="Enter your translation here..."
                  disabled={showAnswer}
                  onKeyDown={e => {
                    if (e.key === 'Enter' && e.ctrlKey && userTranslation.trim() && !showAnswer) {
                      checkTranslation()
                    }
                  }}
                />
              </div>

              {feedback && (
                <Alert
                  className={
                    feedback === 'correct'
                      ? 'border-green-500 bg-green-50'
                      : 'border-red-500 bg-red-50'
                  }
                >
                  <div className="flex items-center gap-2">
                    {feedback === 'correct' ? (
                      <CheckCircle className="h-4 w-4 text-green-600" />
                    ) : (
                      <XCircle className="h-4 w-4 text-red-600" />
                    )}
                    <AlertDescription
                      className={feedback === 'correct' ? 'text-green-800' : 'text-red-800'}
                    >
                      {feedback === 'correct'
                        ? 'Correct! Well done!'
                        : 'Not quite right. Try again!'}
                    </AlertDescription>
                  </div>
                </Alert>
              )}

              {showAnswer && (
                <div className="space-y-4">
                  <div className="p-4 bg-blue-50 rounded-lg border border-blue-200">
                    <div className="font-semibold text-blue-900 mb-1">Correct Answer:</div>
                    <div className="text-blue-800">{currentSentence.english}</div>
                  </div>

                  {histories.length > 0 && (
                    <div className="p-4 bg-yellow-50 rounded-lg border border-yellow-200">
                      <div className="font-semibold text-yellow-900 mb-2">
                        Previous Incorrect Answers:
                      </div>
                      <ul className="text-yellow-800 space-y-1">
                        {histories.map(history => (
                          <li key={history.id} className="text-sm">
                            &ldquo;{history.incorrect_answer}&rdquo;
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              )}
            </CardContent>
            <CardFooter className="flex gap-2">
              {!showAnswer ? (
                <Button
                  onClick={checkTranslation}
                  disabled={!userTranslation.trim()}
                  className="flex-1 bg-gray-500 hover:bg-black text-white"
                >
                  Check Translation
                </Button>
              ) : (
                <Button onClick={nextSentence} className="flex-1">
                  Next Sentence
                </Button>
              )}
            </CardFooter>
          </Card>
        </div>
      </div>
    </div>
  )
}
