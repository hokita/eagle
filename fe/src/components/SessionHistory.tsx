'use client'

import { useEffect, useRef, useState } from 'react'
import type { User } from 'firebase/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import AppHeader from './AppHeader'
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
          <div className="space-y-3">
            {sessions.map(session => {
              const detail = openId === session.id ? details[session.id] : undefined
              return (
                <Card key={session.id}>
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
                    {openId === session.id && detailErrors[session.id] && (
                      <div className="space-y-2 border-t border-border pt-3 text-sm">
                        <p className="text-foreground">Failed to load the session.</p>
                        <Button onClick={() => fetchDetail(session.id)} className="w-full">
                          Try Again
                        </Button>
                      </div>
                    )}
                    {detail && (
                      <div className="space-y-3 border-t border-border pt-3 text-sm">
                        <div>
                          <p className="text-xs font-semibold text-muted-foreground">Conversation</p>
                          {detail.transcript.map((message, i) => (
                            <p key={i} className="text-foreground">
                              <span className="text-muted-foreground">
                                {message.role === 'user' ? 'You: ' : 'AI: '}
                              </span>
                              {message.text}
                            </p>
                          ))}
                        </div>
                        {detail.reflection_ja && (
                          <div>
                            <p className="text-xs font-semibold text-muted-foreground">Reflection</p>
                            <p className="text-foreground">{detail.reflection_ja}</p>
                          </div>
                        )}
                        {/* Sessions recorded before the summary replaced the
                            study/retry flow have neither field. */}
                        {detail.natural_english && (
                          <div>
                            <p className="text-xs font-semibold text-muted-foreground">
                              Natural English
                            </p>
                            <p className="text-foreground">{detail.natural_english}</p>
                          </div>
                        )}
                        {(detail.naturalness_why_en || detail.naturalness_fix_en) && (
                          <div>
                            <p className="text-xs font-semibold text-muted-foreground">
                              Why it sounded unnatural
                            </p>
                            {detail.naturalness_why_en && (
                              <p className="text-foreground">{detail.naturalness_why_en}</p>
                            )}
                            {detail.naturalness_fix_en && (
                              <p className="mt-1 text-foreground">{detail.naturalness_fix_en}</p>
                            )}
                          </div>
                        )}
                        {detail.phrases.length > 0 && (
                          <div>
                            <p className="text-xs font-semibold text-muted-foreground">
                              Useful phrases
                            </p>
                            <div className="flex flex-wrap gap-1.5">
                              {detail.phrases.map(phrase => (
                                <span
                                  key={phrase.phrase}
                                  className="rounded-md border border-border bg-muted px-2 py-1 text-xs text-foreground"
                                >
                                  {phrase.phrase}
                                </span>
                              ))}
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </CardContent>
                </Card>
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
