'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import type { User } from 'firebase/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import AppHeader from './AppHeader'
import SettingsSheet from './SettingsSheet'
import ChatTranscript from './ChatTranscript'
import ReflectionPrompt from './ReflectionPrompt'
import GapAndExpressions from './GapAndExpressions'
import RetryForm from './RetryForm'
import ComparisonView from './ComparisonView'
import { useSettings } from '@/lib/useSettings'
import {
  api,
  type DiscussionQuestion,
  type DiscussionMessage,
  type GapAnalysis,
  type DiscussionCompleteResponse,
} from '@/lib/api'

type Phase = 'loading' | 'conversation' | 'reflection' | 'studying' | 'retry' | 'comparison'

interface Props {
  user: User
}

export default function DiscussionSession({ user }: Props) {
  const { levels, language, setLevels, setLanguage } = useSettings()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const [phase, setPhase] = useState<Phase>('loading')
  const [question, setQuestion] = useState<DiscussionQuestion | null>(null)
  const [transcript, setTranscript] = useState<DiscussionMessage[]>([])
  const [analysis, setAnalysis] = useState<GapAnalysis | null>(null)
  const [reflectionJa, setReflectionJa] = useState('')
  const [result, setResult] = useState<DiscussionCompleteResponse | null>(null)
  const [retryAnswer, setRetryAnswer] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadQuestion = async () => {
    setPhase('loading')
    setError(null)
    try {
      const q = await api.getDiscussionQuestion()
      setQuestion(q)
      setPhase('conversation')
    } catch (err) {
      if (err instanceof Error && err.message === 'API error: 404') {
        setError('No discussion questions available yet.')
      } else {
        setError('Failed to load a question.')
      }
    }
  }

  useEffect(() => {
    loadQuestion()
  }, [])

  const requestReply = async (current: DiscussionMessage[]) => {
    if (!question) return
    setBusy(true)
    setError(null)
    try {
      const reply = await api.discussionReply(question.id, current)
      if (reply.done) {
        if (reply.message) {
          setTranscript([...current, { role: 'ai', text: reply.message }])
        }
        setPhase('reflection')
      } else {
        setTranscript([...current, { role: 'ai', text: reply.message }])
      }
    } catch {
      setError('Something went wrong. Please try again.')
    } finally {
      setBusy(false)
    }
  }

  const sendMessage = (text: string) => {
    // A previous reply may have failed after the user's turn was already
    // appended, leaving the transcript ending in an unanswered "user"
    // message. Sending again must replace that pending turn rather than
    // appending a second consecutive user message, which the server
    // rejects (and would brick the session on every later call).
    const last = transcript[transcript.length - 1]
    const next: DiscussionMessage[] =
      last && last.role === 'user'
        ? [...transcript.slice(0, -1), { role: 'user', text }]
        : [...transcript, { role: 'user', text }]
    setTranscript(next)
    requestReply(next)
  }

  const submitReflection = async (text: string) => {
    if (!question) return
    setBusy(true)
    setError(null)
    try {
      const gap = await api.discussionAnalyze(question.id, transcript, text)
      setReflectionJa(text)
      setAnalysis(gap)
      setPhase('studying')
    } catch {
      setError('Something went wrong. Please try again.')
    } finally {
      setBusy(false)
    }
  }

  const skipReflection = () => {
    setReflectionJa('')
    setAnalysis(null)
    // Skipping intentionally abandons any failed analyze attempt — a stale
    // error banner must not follow the learner into the retry phase.
    setError(null)
    setPhase('retry')
  }

  const submitRetry = async (text: string) => {
    if (!question) return
    setBusy(true)
    setError(null)
    setRetryAnswer(text)
    try {
      const res = await api.discussionComplete({
        question_id: question.id,
        transcript,
        reflection_ja: reflectionJa,
        expressed_ideas: analysis?.expressed_ideas ?? [],
        missing_ideas: analysis?.missing_ideas ?? [],
        expressions: analysis?.expressions ?? [],
        retry_answer: text,
      })
      setResult(res)
      setPhase('comparison')
    } catch {
      setError('Something went wrong. Please try again.')
    } finally {
      setBusy(false)
    }
  }

  const restart = () => {
    setQuestion(null)
    setTranscript([])
    setAnalysis(null)
    setReflectionJa('')
    setResult(null)
    setRetryAnswer('')
    loadQuestion()
  }

  // The conversation error is the only one whose input (the sent message) is
  // already consumed — recover by re-requesting a reply for the transcript
  // as it stands. Reflection/retry keep their drafts, so plain resubmission
  // covers those.
  const canRetryReply =
    phase === 'conversation' &&
    transcript.length > 0 &&
    transcript[transcript.length - 1].role === 'user'

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <AppHeader
          user={user}
          onOpenSettings={() => setSettingsOpen(true)}
          showDiscussionLink={false}
        />
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-bold text-foreground">Discussion</h2>
          <Link
            href="/discussion/history"
            className="rounded-md px-2 py-1 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            History
          </Link>
        </div>

        {error && (
          <Card className="mb-3">
            <CardContent className="pt-6 space-y-2">
              <p className="text-sm text-destructive">{error}</p>
              {phase === 'loading' && (
                <Button variant="outline" size="sm" onClick={loadQuestion}>
                  Try Again
                </Button>
              )}
              {canRetryReply && (
                <Button variant="outline" size="sm" onClick={() => requestReply(transcript)}>
                  Try Again
                </Button>
              )}
            </CardContent>
          </Card>
        )}

        {phase === 'loading' && !error && (
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-muted-foreground">Loading...</p>
          </div>
        )}

        {phase === 'conversation' && question && (
          <ChatTranscript
            question={question.question_en}
            transcript={transcript}
            sending={busy}
            canFinish={transcript.length >= 3}
            onSend={sendMessage}
            onFinish={() => setPhase('reflection')}
          />
        )}

        {phase === 'reflection' && (
          <div className="space-y-3">
            {transcript.length > 0 && transcript[transcript.length - 1].role === 'ai' && (
              <Card>
                <CardContent className="pt-6">
                  <p className="text-sm text-foreground">
                    {transcript[transcript.length - 1].text}
                  </p>
                </CardContent>
              </Card>
            )}
            <ReflectionPrompt loading={busy} onSubmit={submitReflection} onSkip={skipReflection} />
          </div>
        )}

        {phase === 'studying' && analysis && (
          <GapAndExpressions analysis={analysis} onContinue={() => setPhase('retry')} />
        )}

        {phase === 'retry' && question && (
          <RetryForm
            question={question.question_en}
            expressions={analysis?.expressions ?? []}
            loading={busy}
            onSubmit={submitRetry}
          />
        )}

        {phase === 'comparison' && result && (
          <ComparisonView
            before={transcript[0]?.text ?? ''}
            after={retryAnswer}
            expressions={analysis?.expressions ?? []}
            feedback={result.retry_feedback}
            onRestart={restart}
          />
        )}
      </div>

      <SettingsSheet
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        levels={levels}
        onLevelsChange={setLevels}
        language={language}
        onLanguageChange={setLanguage}
      />
    </div>
  )
}
