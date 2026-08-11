'use client'

import { useState, useEffect, useRef } from 'react'
import type { User } from 'firebase/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import AppHeader from './AppHeader'
import SettingsSheet from './SettingsSheet'
import QuestionCard from './QuestionCard'
import ReviewPanel, { type ReviewTab } from './ReviewPanel'
import { api, type Sentence, type AnswerHistory } from '@/lib/api'
import { speakJapanese } from '@/lib/speech'
import {
  useSettings,
  loadStoredLevels,
  levelsForRequest,
  levelSummary,
  type ExplainLanguage,
} from '@/lib/useSettings'

interface Props {
  user: User
}

export default function Translator({ user }: Props) {
  const { levels, language, setLevels, setLanguage } = useSettings()

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
  const [tab, setTab] = useState<ReviewTab>('answer')
  const [settingsOpen, setSettingsOpen] = useState(false)

  const latestRequestId = useRef(0)
  const explainRequestId = useRef(0)

  const getRandomSentence = async (levelsOverride?: number[]) => {
    const requestId = ++latestRequestId.current
    try {
      setLoading(true)
      setError(null)
      const sentence = await api.getRandomSentence(
        levelsForRequest(levelsOverride ?? levels)
      )
      if (requestId !== latestRequestId.current) return
      setCurrentSentence(sentence)
      setCorrectCount(sentence.correct_count)
      setIncorrectCount(sentence.incorrect_count)
    } catch (err) {
      if (requestId !== latestRequestId.current) return
      setError(err instanceof Error ? err.message : 'Failed to load sentence')
    } finally {
      if (requestId === latestRequestId.current) setLoading(false)
    }
  }

  useEffect(() => {
    getRandomSentence(loadStoredLevels())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const resetQuestionState = () => {
    explainRequestId.current++
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
    setTab('answer')
    if ('speechSynthesis' in window) speechSynthesis.cancel()
  }

  const handleLevelsChange = (next: number[]) => {
    setLevels(next)
    resetQuestionState()
    getRandomSentence(next)
  }

  const handleLanguageChange = (next: ExplainLanguage) => {
    setLanguage(next)
    if (explanation || explainError || explaining) explainAnswer(next)
  }

  const explainAnswer = async (lang: ExplainLanguage) => {
    if (!currentSentence) return
    const requestId = ++explainRequestId.current
    setExplaining(true)
    setExplainError(null)
    setExplanation(null)
    try {
      const result = await api.explainAnswer(currentSentence.id, userTranslation, lang)
      if (requestId !== explainRequestId.current) return
      setExplanation(result.explanation)
    } catch (err) {
      if (requestId !== explainRequestId.current) return
      setExplainError(err instanceof Error ? err.message : 'Failed to load explanation')
    } finally {
      if (requestId === explainRequestId.current) setExplaining(false)
    }
  }

  const handleTabChange = (next: ReviewTab) => {
    setTab(next)
    if (next === 'explain' && !explanation && !explaining && !explainError) {
      explainAnswer(language)
    }
  }

  const checkTranslation = async () => {
    if (!currentSentence) return
    try {
      const result = await api.checkAnswer(currentSentence.id, userTranslation.trim())
      setFeedback(result.is_correct ? 'correct' : 'incorrect')
      setHistories(result.histories)
      setShowAnswer(true)
      setTab('answer')
      if (result.is_correct) setCorrectCount(prev => prev + 1)
      else setIncorrectCount(prev => prev + 1)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to check answer')
    }
  }

  const nextSentence = () => {
    resetQuestionState()
    getRandomSentence()
  }

  const reportSentence = async (sentenceId: number) => {
    try {
      await api.reportSentence(sentenceId)
      setIsReported(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to report sentence')
    }
  }

  const capitalizeFirstLetter = (text: string) => text.charAt(0).toUpperCase() + text.slice(1)

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="mx-auto max-w-2xl">
        <AppHeader user={user} onOpenSettings={() => setSettingsOpen(true)} />

        {loading ? (
          <div className="text-center">
            <div className="mx-auto mb-4 h-12 w-12 animate-spin rounded-full border-b-2 border-indigo-600" />
            <p className="text-muted-foreground">Loading...</p>
          </div>
        ) : error || !currentSentence ? (
          <Card>
            <CardContent className="p-5">
              <p className="mb-4 text-foreground">{error || 'Failed to load content'}</p>
              <Button onClick={() => getRandomSentence()} className="w-full">
                Try Again
              </Button>
            </CardContent>
          </Card>
        ) : (
          <>
            <div className="space-y-3">
              <QuestionCard
                sentence={currentSentence}
                correctCount={correctCount}
                incorrectCount={incorrectCount}
                levelSummary={levelSummary(levels)}
                isSpeaking={isSpeaking}
                onSpeak={() => speakJapanese(currentSentence.japanese, setIsSpeaking)}
              />

              {!showAnswer ? (
                <Card>
                  <CardContent className="p-5">
                    <form
                      onSubmit={e => {
                        e.preventDefault()
                        if (userTranslation.trim()) checkTranslation()
                      }}
                      className="space-y-2"
                    >
                      <Label htmlFor="translation">Your translation</Label>
                      <Textarea
                        id="translation"
                        value={userTranslation}
                        onChange={e => setUserTranslation(e.target.value)}
                        placeholder="Enter your translation here..."
                        onBlur={e => {
                          if (e.target.value.trim()) {
                            setUserTranslation(capitalizeFirstLetter(e.target.value.trim()))
                          }
                        }}
                        onKeyDown={e => {
                          if (e.key === 'Enter' && e.ctrlKey && userTranslation.trim()) {
                            checkTranslation()
                          }
                        }}
                        aria-label="Your English translation"
                        aria-required="true"
                      />
                    </form>
                  </CardContent>
                </Card>
              ) : (
                feedback && (
                  <ReviewPanel
                    feedback={feedback}
                    userAnswer={userTranslation}
                    correctAnswer={currentSentence.english}
                    histories={histories}
                    tab={tab}
                    onTabChange={handleTabChange}
                    explanation={explanation}
                    explaining={explaining}
                    explainError={explainError}
                    onRetryExplain={() => explainAnswer(language)}
                  />
                )
              )}
            </div>

            <div className="sticky bottom-0 bg-background/80 pb-4 pt-3 backdrop-blur-sm">
              {!showAnswer ? (
                <Button
                  onClick={checkTranslation}
                  disabled={!userTranslation.trim()}
                  className="w-full"
                >
                  Check Translation
                </Button>
              ) : (
                <div className="flex gap-2">
                  <Button onClick={nextSentence} className="flex-1">
                    Next Sentence
                  </Button>
                  <Button
                    variant="ghost"
                    onClick={() => reportSentence(currentSentence.id)}
                    disabled={isReported}
                  >
                    {isReported ? 'Reported' : 'Report'}
                  </Button>
                </div>
              )}
            </div>
          </>
        )}

        <SettingsSheet
          open={settingsOpen}
          onClose={() => setSettingsOpen(false)}
          levels={levels}
          onLevelsChange={handleLevelsChange}
          language={language}
          onLanguageChange={handleLanguageChange}
        />
      </div>
    </div>
  )
}
