'use client'

import { useEffect, useRef, useState } from 'react'
import type { User } from 'firebase/auth'
import ReactMarkdown from 'react-markdown'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import AppHeader from './AppHeader'
import SettingsSheet from './SettingsSheet'
import { api, type Mistake } from '@/lib/api'
import { auth } from '@/lib/firebase'
import { useSettings, loadStoredLanguage } from '@/lib/useSettings'

// insightCacheKey scopes the cached insight to the viewing user, the
// language it was generated in (so switching accounts or explain-language on
// a shared browser never serves someone else's, or the wrong language's,
// cached analysis), and a fingerprint of the mistake history. Mistakes only
// ever accumulate (a sentence's IncorrectCount never resets), and the list is
// sorted most-recently-missed first, so the most recent wrong answer's id
// (a strictly increasing timestamp) changes the instant a new mistake is
// recorded — any change to it naturally misses the old cache entry instead
// of serving a stale insight generated before that mistake happened.
function insightCacheKey(language: string, mistakes: Mistake[]): string | null {
  const uid = auth.currentUser?.uid
  if (!uid) return null
  const fingerprint = mistakes[0]?.wrong_answers[0]?.id ?? 0
  return `eagle:mistakesInsight:${uid}:${language}:${fingerprint}`
}

const insightMarkdownComponents = {
  p: (props: React.ComponentPropsWithoutRef<'p'>) => <p className="mb-2 last:mb-0" {...props} />,
  ul: (props: React.ComponentPropsWithoutRef<'ul'>) => (
    <ul className="list-disc pl-5 space-y-1 mb-2 last:mb-0" {...props} />
  ),
  ol: (props: React.ComponentPropsWithoutRef<'ol'>) => (
    <ol className="list-decimal pl-5 space-y-1 mb-2 last:mb-0" {...props} />
  ),
  li: (props: React.ComponentPropsWithoutRef<'li'>) => <li {...props} />,
  strong: (props: React.ComponentPropsWithoutRef<'strong'>) => (
    <strong className="font-semibold text-foreground" {...props} />
  ),
}

interface Props {
  user: User
}

export default function Mistakes({ user }: Props) {
  const { levels, language, setLevels, setLanguage } = useSettings()
  const [mistakes, setMistakes] = useState<Mistake[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [insight, setInsight] = useState<string | null>(null)
  const [insightLoading, setInsightLoading] = useState(false)
  const [insightError, setInsightError] = useState<string | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)

  const insightRequestId = useRef(0)

  const loadInsight = async (currentMistakes: Mistake[], language: 'en' | 'ja') => {
    const requestId = ++insightRequestId.current
    const cacheKey = insightCacheKey(language, currentMistakes)
    setInsightError(null)

    if (cacheKey) {
      const cached = sessionStorage.getItem(cacheKey)
      if (cached !== null) {
        if (requestId !== insightRequestId.current) return
        setInsight(cached)
        return
      }
    }

    setInsightLoading(true)
    try {
      const result = await api.getMistakesInsight(language)
      if (requestId !== insightRequestId.current) return
      setInsight(result.insight)
      if (cacheKey) {
        sessionStorage.setItem(cacheKey, result.insight)
      }
    } catch (err) {
      if (requestId !== insightRequestId.current) return
      setInsightError(err instanceof Error ? err.message : 'Failed to load insight')
    } finally {
      if (requestId === insightRequestId.current) setInsightLoading(false)
    }
  }

  const loadMistakes = async () => {
    try {
      setLoading(true)
      setError(null)
      const result = await api.listMistakes()
      setMistakes(result.mistakes)
      if (result.mistakes.length > 0) {
        loadInsight(result.mistakes, loadStoredLanguage())
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load mistakes')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadMistakes()
  }, [])

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <AppHeader user={user} onOpenSettings={() => setSettingsOpen(true)} showMistakesLink={false} />
        <h2 className="mb-4 text-lg font-bold text-foreground">Mistakes</h2>

        {loading ? (
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-muted-foreground">Loading...</p>
          </div>
        ) : error ? (
          <Card>
            <CardContent className="pt-6">
              <p className="text-foreground mb-4">{error}</p>
              <Button onClick={loadMistakes} className="w-full">
                Try Again
              </Button>
            </CardContent>
          </Card>
        ) : mistakes && mistakes.length === 0 ? (
          <Card>
            <CardContent className="pt-6 text-center text-muted-foreground">
              No mistakes yet — nice work!
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-3">
            {(insightLoading || insight || insightError) && (
              <Card>
                <CardHeader className="pb-2 flex-row items-center justify-between space-y-0">
                  <CardTitle className="text-base">Weakness Insight</CardTitle>
                </CardHeader>
                <CardContent>
                  {insightLoading ? (
                    <p className="text-sm text-muted-foreground">Analyzing your mistakes…</p>
                  ) : insightError ? (
                    <div className="space-y-2">
                      <p className="text-sm text-destructive">{insightError}</p>
                      <Button variant="outline" size="sm" onClick={() => loadInsight(mistakes ?? [], language)}>
                        Try Again
                      </Button>
                    </div>
                  ) : (
                    <div className="text-sm text-foreground">
                      <ReactMarkdown components={insightMarkdownComponents} disallowedElements={['a', 'img']}>
                        {insight}
                      </ReactMarkdown>
                    </div>
                  )}
                </CardContent>
              </Card>
            )}
            {mistakes?.map(mistake => (
              <Card key={mistake.sentence_id}>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base">{mistake.japanese}</CardTitle>
                </CardHeader>
                <CardContent className="space-y-2">
                  <div className="inline-block rounded-md border border-border bg-muted px-2 py-1 text-sm text-foreground">
                    {mistake.correct_answer}
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {mistake.wrong_answers.map(wrong => (
                      <span
                        key={wrong.id}
                        className="rounded-md border border-border bg-muted px-2 py-1 text-xs text-muted-foreground line-through"
                      >
                        {wrong.incorrect_answer}
                      </span>
                    ))}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>

      <SettingsSheet
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        levels={levels}
        onLevelsChange={setLevels}
        language={language}
        onLanguageChange={next => {
          setLanguage(next)
          if (mistakes?.length) loadInsight(mistakes, next)
        }}
      />
    </div>
  )
}
