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
import SummaryView from './SummaryView'
import { useSettings } from '@/lib/useSettings'
import {
  api,
  type DiscussionQuestion,
  type DiscussionMessage,
  type DiscussionCompleteResponse,
} from '@/lib/api'

type Phase = 'loading' | 'conversation' | 'reflection' | 'summary'

interface Props {
  user: User
}

export default function DiscussionSession({ user }: Props) {
  const { levels, language, setLevels, setLanguage } = useSettings()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const [phase, setPhase] = useState<Phase>('loading')
  const [question, setQuestion] = useState<DiscussionQuestion | null>(null)
  const [transcript, setTranscript] = useState<DiscussionMessage[]>([])
  const [result, setResult] = useState<DiscussionCompleteResponse | null>(null)
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
      // A done reply carries no message — the server ends the conversation
      // once both follow-ups are answered, so there is nothing to append.
      if (reply.done) {
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

  // The reflection is the last thing the learner writes: submitting it
  // summarizes and saves the session in one call, so there is no step left
  // between here and the summary screen.
  const submitReflection = async (text: string) => {
    if (!question) return
    setBusy(true)
    setError(null)
    try {
      const res = await api.discussionComplete(question.id, transcript, text)
      setResult(res)
      setPhase('summary')
    } catch {
      setError('Something went wrong. Please try again.')
    } finally {
      setBusy(false)
    }
  }

  const restart = () => {
    setQuestion(null)
    setTranscript([])
    setResult(null)
    loadQuestion()
  }

  // The conversation error is the only one whose input (the sent message) is
  // already consumed — recover by re-requesting a reply for the transcript
  // as it stands. The reflection keeps its draft, so plain resubmission
  // covers that one.
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
            onSend={sendMessage}
          />
        )}

        {phase === 'reflection' && (
          <ReflectionPrompt loading={busy} onSubmit={submitReflection} />
        )}

        {phase === 'summary' && result && question && (
          <SummaryView
            question={question.question_en}
            transcript={transcript}
            naturalEnglish={result.natural_english}
            naturalnessWhyEn={result.naturalness_why_en}
            naturalnessFixEn={result.naturalness_fix_en}
            phrases={result.phrases}
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
