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
import { CheckCircle, XCircle, Volume2 } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import Image from 'next/image'
import type { User } from 'firebase/auth'
import { api, type Sentence, type AnswerHistory } from '@/lib/api'
import UserMenu from './UserMenu'

interface Props {
  user: User
}

export default function Translator({ user }: Props) {
  const [currentSentence, setCurrentSentence] = useState<Sentence | null>(null)
  const [userTranslation, setUserTranslation] = useState('')
  const [feedback, setFeedback] = useState<'correct' | 'incorrect' | null>(null)
  const [showAnswer, setShowAnswer] = useState(false)
  const [loading, setLoading] = useState(true)
  const [histories, setHistories] = useState<AnswerHistory[]>([])
  const [error, setError] = useState<string | null>(null)
  const [correctCount, setCorrectCount] = useState(0)
  const [incorrectCount, setIncorrectCount] = useState(0)
  const [isReported, setIsReported] = useState(false)
  const [isSpeaking, setIsSpeaking] = useState(false)
  const [explanation, setExplanation] = useState<string | null>(null)
  const [explaining, setExplaining] = useState(false)
  const [explainError, setExplainError] = useState<string | null>(null)
  const [level, setLevel] = useState(0)

  const speakJapanese = (text: string) => {
    if ('speechSynthesis' in window) {
      speechSynthesis.cancel()
      setIsSpeaking(true)

      const speak = () => {
        const utterance = new SpeechSynthesisUtterance(text)

        // Try to find a Japanese voice
        const voices = speechSynthesis.getVoices()
        const japaneseVoice = voices.find(voice =>
          voice.lang.startsWith('ja') || voice.lang.includes('JP')
        )

        if (japaneseVoice) {
          utterance.voice = japaneseVoice
        }

        utterance.lang = 'ja-JP'
        utterance.rate = 0.8
        utterance.pitch = 1
        utterance.volume = 1

        utterance.onstart = () => {
          setIsSpeaking(true)
        }

        utterance.onend = () => {
          setIsSpeaking(false)
        }

        utterance.onerror = () => {
          setIsSpeaking(false)
        }

        speechSynthesis.speak(utterance)

        // Safari workaround: Force reset if no speech after 500ms
        setTimeout(() => {
          if (!speechSynthesis.speaking && !speechSynthesis.pending) {
            setIsSpeaking(false)
          }
        }, 500)
      }

      // Wait for voices to load
      setTimeout(() => {
        speak()
      }, 100)
    }
  }

  const reportSentence = async (sentenceId: number) => {
    try {
      await api.reportSentence(sentenceId)
      setIsReported(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to report sentence')
    }
  }

  const explainAnswer = async () => {
    if (!currentSentence) return
    setExplaining(true)
    setExplainError(null)
    try {
      const result = await api.explainAnswer(currentSentence.id, userTranslation)
      setExplanation(result.explanation)
    } catch (err) {
      setExplainError(err instanceof Error ? err.message : 'Failed to load explanation')
    } finally {
      setExplaining(false)
    }
  }

  const getRandomSentence = async (levelOverride?: number) => {
    try {
      setLoading(true)
      setError(null)
      const sentence = await api.getRandomSentence(levelOverride ?? level)
      setCurrentSentence(sentence)
      setCorrectCount(sentence.correct_count)
      setIncorrectCount(sentence.incorrect_count)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sentence')
    } finally {
      setLoading(false)
    }
  }

  const capitalizeFirstLetter = (text: string) => {
    return text.charAt(0).toUpperCase() + text.slice(1)
  }

  const checkTranslation = async () => {
    if (!currentSentence) return

    const trimmedUserTranslation = userTranslation.trim()

    try {
      const result = await api.checkAnswer(currentSentence.id, trimmedUserTranslation)
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

  const resetQuestionState = () => {
    setUserTranslation('')
    setFeedback(null)
    setShowAnswer(false)
    setHistories([])
    setError(null)
    setCorrectCount(0)
    setIncorrectCount(0)
    setIsReported(false)
    setIsSpeaking(false)
    setExplanation(null)
    setExplaining(false)
    setExplainError(null)
    if ('speechSynthesis' in window) {
      speechSynthesis.cancel()
    }
  }

  const nextSentence = () => {
    resetQuestionState()
    getRandomSentence()
  }

  const handleLevelChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const newLevel = Number(e.target.value)
    setLevel(newLevel)
    resetQuestionState()
    getRandomSentence(newLevel)
  }

  useEffect(() => {
    getRandomSentence()
  }, [])

  const levelSelector = (
    <select
      aria-label="Sentence difficulty level"
      value={level}
      onChange={handleLevelChange}
      className="h-9 rounded-md border border-input bg-background px-2 text-sm"
    >
      <option value={0}>Any level</option>
      <option value={1}>1</option>
      <option value={2}>2</option>
      <option value={3}>3</option>
      <option value={4}>4</option>
      <option value={5}>5</option>
    </select>
  )

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <div className="mb-8">
          <div className="flex items-center justify-end mb-2 gap-2">
            {levelSelector}
            <UserMenu user={user} />
          </div>
          <div className="flex items-center justify-center gap-2">
            <Image src="/eagle-thumbnail.png" alt="Eagle logo" width={32} height={32} />
            <h1 className="text-3xl font-bold text-gray-900">Eagle</h1>
          </div>
        </div>

        {loading ? (
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-gray-600">Loading...</p>
          </div>
        ) : error || !currentSentence ? (
          <Card className="max-w-md mx-auto">
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
        ) : (
        <div className="grid gap-6 mb-6">
          <Card>
            <CardHeader>
              <CardTitle>Translate this sentence</CardTitle>
              <CardDescription>Translate the Japanese sentence below into English</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="text-center">
                <div className="flex items-center justify-center gap-3 mb-2">
                  <div className="text-3xl font-bold text-gray-900">
                    {currentSentence.japanese}
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => speakJapanese(currentSentence.japanese)}
                    disabled={isSpeaking}
                    className="flex items-center px-2 py-1"
                  >
                    <Volume2 className="h-3 w-3" />
                  </Button>
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

              <form
                onSubmit={e => {
                  e.preventDefault()
                  if (userTranslation.trim() && !showAnswer) {
                    checkTranslation()
                  }
                }}
                className="space-y-4"
              >
                <div className="space-y-2">
                  <Label htmlFor="translation">Your English translation:</Label>
                  <Textarea
                    id="translation"
                    value={userTranslation}
                    onChange={e => setUserTranslation(e.target.value)}
                    placeholder="Enter your translation here..."
                    disabled={showAnswer}
                    onBlur={e => {
                      if (e.target.value.trim() && !showAnswer) {
                        const capitalizedTranslation = capitalizeFirstLetter(e.target.value.trim())
                        setUserTranslation(capitalizedTranslation)
                      }
                    }}
                    onKeyDown={e => {
                      if (e.key === 'Enter' && e.ctrlKey && userTranslation.trim() && !showAnswer) {
                        checkTranslation()
                      }
                    }}
                    aria-label="Your English translation"
                    aria-required="true"
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
                        className={
                          feedback === 'correct' ? 'text-green-800' : 'text-red-800'
                        }
                      >
                        {feedback === 'correct'
                          ? 'Correct! Well done!'
                          : 'Not quite right. Try again!'}
                      </AlertDescription>
                    </div>
                  </Alert>
                )}

                {!showAnswer && (
                  <Button
                    type="submit"
                    disabled={!userTranslation.trim()}
                    className="w-full bg-gray-500 hover:bg-black text-white"
                  >
                    Check Translation
                  </Button>
                )}
              </form>

              {showAnswer && (
                <div className="space-y-4">
                  <div className="p-4 bg-blue-50 rounded-lg border border-blue-200">
                    <div className="font-semibold text-blue-900 mb-1">
                      Correct Answer:
                    </div>
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

                  {feedback === 'incorrect' && (
                    <div className="space-y-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={explainAnswer}
                        disabled={explaining}
                      >
                        {explaining ? 'Explaining...' : 'Explain'}
                      </Button>

                      {explainError && (
                        <Alert className="border-red-500 bg-red-50">
                          <AlertDescription className="text-red-800">
                            {explainError}
                          </AlertDescription>
                        </Alert>
                      )}

                      {explanation && (
                        <div className="p-4 bg-purple-50 rounded-lg border border-purple-200">
                          <div className="font-semibold text-purple-900 mb-1">Explanation:</div>
                          <div className="text-purple-800 whitespace-pre-wrap">{explanation}</div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}
            </CardContent>
            <CardFooter className="flex gap-2">
              {showAnswer && (
                <>
                  <Button onClick={nextSentence} className="flex-1">
                    Next Sentence
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      if (currentSentence) {
                        reportSentence(currentSentence.id)
                      }
                    }}
                    disabled={isReported}
                  >
                    {isReported ? 'Reported' : 'Report'}
                  </Button>
                </>
              )}
            </CardFooter>

          </Card>
        </div>
        )}
      </div>
    </div>
  )
}
