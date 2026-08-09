'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import ReactMarkdown from 'react-markdown'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { api, type Mistake } from '@/lib/api'
import { auth } from '@/lib/firebase'

const EXPLAIN_LANGUAGE_STORAGE_KEY = 'eagle:explainLanguage'

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
    <strong className="font-semibold text-indigo-900" {...props} />
  ),
}

export default function Mistakes() {
  const [mistakes, setMistakes] = useState<Mistake[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [insight, setInsight] = useState<string | null>(null)
  const [insightLoading, setInsightLoading] = useState(false)
  const [insightError, setInsightError] = useState<string | null>(null)
  const [insightLanguage, setInsightLanguage] = useState<'en' | 'ja'>(() => {
    const stored = typeof window !== 'undefined' ? localStorage.getItem(EXPLAIN_LANGUAGE_STORAGE_KEY) : null
    return stored === 'ja' ? 'ja' : 'en'
  })

  const loadInsight = async (currentMistakes: Mistake[], language: 'en' | 'ja') => {
    const cacheKey = insightCacheKey(language, currentMistakes)

    if (cacheKey) {
      const cached = sessionStorage.getItem(cacheKey)
      if (cached !== null) {
        setInsight(cached)
        return
      }
    }

    setInsightLoading(true)
    setInsightError(null)
    try {
      const result = await api.getMistakesInsight(language)
      setInsight(result.insight)
      if (cacheKey) {
        sessionStorage.setItem(cacheKey, result.insight)
      }
    } catch (err) {
      setInsightError(err instanceof Error ? err.message : 'Failed to load insight')
    } finally {
      setInsightLoading(false)
    }
  }

  const selectInsightLanguage = (language: 'en' | 'ja') => {
    setInsightLanguage(language)
    if (typeof window !== 'undefined') {
      localStorage.setItem(EXPLAIN_LANGUAGE_STORAGE_KEY, language)
    }
    if (mistakes && mistakes.length > 0) {
      loadInsight(mistakes, language)
    }
  }

  const loadMistakes = async () => {
    try {
      setLoading(true)
      setError(null)
      const result = await api.listMistakes()
      setMistakes(result.mistakes)
      if (result.mistakes.length > 0) {
        loadInsight(result.mistakes, insightLanguage)
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
        <div className="flex items-center gap-3 mb-6">
          <Button asChild variant="outline" size="sm">
            <Link href="/">&larr; Back</Link>
          </Button>
          <h1 className="text-2xl font-bold text-gray-900">Mistakes</h1>
        </div>

        {loading ? (
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-gray-600">Loading...</p>
          </div>
        ) : error ? (
          <Card>
            <CardContent className="pt-6">
              <p className="text-gray-700 mb-4">{error}</p>
              <Button onClick={loadMistakes} className="w-full">
                Try Again
              </Button>
            </CardContent>
          </Card>
        ) : mistakes && mistakes.length === 0 ? (
          <Card>
            <CardContent className="pt-6 text-center text-gray-600">
              No mistakes yet — nice work!
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-3">
            {(insightLoading || insight || insightError) && (
              <Card className="border-indigo-300">
                <CardHeader className="pb-2 flex-row items-center justify-between space-y-0">
                  <CardTitle className="text-base text-indigo-900">Weakness Insight</CardTitle>
                  <div className="flex gap-1" role="group" aria-label="Insight language">
                    <Button
                      type="button"
                      variant={insightLanguage === 'en' ? 'default' : 'outline'}
                      size="sm"
                      aria-pressed={insightLanguage === 'en'}
                      onClick={() => selectInsightLanguage('en')}
                      disabled={insightLoading}
                    >
                      EN
                    </Button>
                    <Button
                      type="button"
                      variant={insightLanguage === 'ja' ? 'default' : 'outline'}
                      size="sm"
                      aria-pressed={insightLanguage === 'ja'}
                      onClick={() => selectInsightLanguage('ja')}
                      disabled={insightLoading}
                    >
                      JA
                    </Button>
                  </div>
                </CardHeader>
                <CardContent>
                  {insightLoading ? (
                    <p className="text-sm text-gray-600">Analyzing your mistakes…</p>
                  ) : insightError ? (
                    <div className="space-y-2">
                      <p className="text-sm text-red-700">{insightError}</p>
                      <Button variant="outline" size="sm" onClick={() => loadInsight(mistakes ?? [], insightLanguage)}>
                        Try Again
                      </Button>
                    </div>
                  ) : (
                    <div className="text-sm text-gray-800">
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
                  <div className="inline-block text-sm text-blue-900 bg-blue-50 border border-blue-200 rounded-md px-2 py-1">
                    {mistake.correct_answer}
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {mistake.wrong_answers.map(wrong => (
                      <span
                        key={wrong.id}
                        className="text-xs text-yellow-900 bg-yellow-50 border border-yellow-200 rounded-md px-2 py-1"
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
    </div>
  )
}
