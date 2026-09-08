'use client'

import { useEffect, useRef, useState } from 'react'
import type { User } from 'firebase/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import AppHeader from './AppHeader'
import SessionSummary from './SessionSummary'
import SettingsSheet from './SettingsSheet'
import { useSettings } from '@/lib/useSettings'
import { api, type DiscussionSessionSummary, type DiscussionSessionDetail } from '@/lib/api'

interface Props {
  user: User
}

export default function SessionHistory({ user }: Props) {
  const { levels, language, setLevels, setLanguage } = useSettings()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const [sessions, setSessions] = useState<DiscussionSessionSummary[] | null>(null)
  const [details, setDetails] = useState<Record<string, DiscussionSessionDetail>>({})
  const [openId, setOpenId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  // Failed detail fetches, keyed by session id. A map (not a single shared
  // value) so overlapping requests stay independent: a slow failure from one
  // card can never surface in, clear, or overwrite the error of another
  // card — even when two failures are in flight at once.
  const [detailErrors, setDetailErrors] = useState<Record<string, true>>({})

  const clearDetailError = (id: string) => {
    setDetailErrors(prev => {
      if (!prev[id]) return prev
      const next = { ...prev }
      delete next[id]
      return next
    })
  }

  const loadSessions = async () => {
    setError(null)
    try {
      const result = await api.listDiscussionSessions()
      setSessions(result.sessions)
    } catch {
      setError('Failed to load sessions.')
    }
  }

  useEffect(() => {
    loadSessions()
  }, [])

  // Monotonic per-session request generation. Closing and reopening a card
  // while its fetch is in flight starts a second request for the same id;
  // only the latest generation may write state, so a superseded response —
  // success or failure — can never contradict the one the user is seeing.
  const detailRequestSeq = useRef<Record<string, number>>({})

  const fetchDetail = async (id: string) => {
    const seq = (detailRequestSeq.current[id] ?? 0) + 1
    detailRequestSeq.current[id] = seq
    clearDetailError(id)
    try {
      const detail = await api.getDiscussionSession(id)
      if (detailRequestSeq.current[id] !== seq) return
      setDetails(prev => ({ ...prev, [id]: detail }))
      clearDetailError(id)
    } catch {
      if (detailRequestSeq.current[id] !== seq) return
      setDetailErrors(prev => ({ ...prev, [id]: true }))
    }
  }

  const toggle = async (id: string) => {
    if (openId === id) {
      setOpenId(null)
      return
    }
    setOpenId(id)
    if (!details[id]) {
      await fetchDetail(id)
    }
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <AppHeader user={user} onOpenSettings={() => setSettingsOpen(true)} />
        <h2 className="mb-4 text-lg font-bold text-foreground">Discussion History</h2>

        {error ? (
          <Card>
            <CardContent className="pt-6 space-y-2">
              <p className="text-foreground">{error}</p>
              <Button onClick={loadSessions} className="w-full">
                Try Again
              </Button>
            </CardContent>
          </Card>
        ) : sessions === null ? (
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-muted-foreground">Loading...</p>
          </div>
        ) : sessions.length === 0 ? (
          <Card>
            <CardContent className="pt-6 text-center text-muted-foreground">
              No sessions yet — try a discussion!
            </CardContent>
          </Card>
        ) : (
          // Sessions are spaced further apart than the cards inside an open
          // one, so an expanded session reads as one group rather than as
          // more list entries.
          <div className="space-y-6">
            {sessions.map(session => {
              const detail = openId === session.id ? details[session.id] : undefined
              return (
                <div key={session.id} className="space-y-3">
                  <Card>
                    <CardContent className="pt-6 space-y-3">
                      <button
                        type="button"
                        onClick={() => toggle(session.id)}
                        aria-label={session.question_en}
                        className="w-full text-left"
                      >
                        <p className="font-semibold text-foreground">{session.question_en}</p>
                        <p className="text-xs text-muted-foreground">
                          <span>{session.topic}</span>
                          {' · '}
                          <span>{new Date(session.created_at).toLocaleDateString()}</span>
                        </p>
                      </button>
                      {/* Inside the heading card, not below it: the failure
                          belongs to the card the learner just opened, and the
                          retry has to stay attached to it while other cards
                          are open. */}
                      {openId === session.id && detailErrors[session.id] && (
                        <div className="space-y-2 border-t border-border pt-3 text-sm">
                          <p className="text-foreground">Failed to load the session.</p>
                          <Button onClick={() => fetchDetail(session.id)} className="w-full">
                            Try Again
                          </Button>
                        </div>
                      )}
                    </CardContent>
                  </Card>
                  {/* The same card stack the learner saw when the session
                      ended — see SessionSummary. The question is left to the
                      heading card above rather than passed down, since it is
                      already the line directly above the conversation. */}
                  {detail && (
                    <SessionSummary
                      transcript={detail.transcript}
                      reflectionJa={detail.reflection_ja}
                      naturalEnglish={detail.natural_english}
                      naturalnessWhyEn={detail.naturalness_why_en}
                      naturalnessFixEn={detail.naturalness_fix_en}
                      phrases={detail.phrases}
                    />
                  )}
                </div>
              )
            })}
          </div>
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
