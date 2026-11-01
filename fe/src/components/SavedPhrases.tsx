'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import type { User } from 'firebase/auth'
import { Bookmark } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import AppHeader from './AppHeader'
import SettingsSheet from './SettingsSheet'
import { api, type SavedPhrase } from '@/lib/api'
import { useSettings } from '@/lib/useSettings'

interface Props {
  user: User
}

const LABEL = 'text-xs font-semibold uppercase tracking-wide text-muted-foreground'

interface CardProps {
  phrase: SavedPhrase
  onUpdated: (phrase: SavedPhrase) => void
  onDeleted: (id: string) => void
}

// One saved phrase: the phrase, the AI's meaning and example sentences. Each
// card keeps its own pending state, so asking for new examples on one phrase
// or confirming a delete never locks the rest of the list.
function PhraseCard({ phrase, onUpdated, onDeleted }: CardProps) {
  const [rewriting, setRewriting] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const written = phrase.meaning !== '' && phrase.examples.length > 0

  const rewrite = async () => {
    setRewriting(true)
    setError(null)
    try {
      onUpdated(await api.rewritePhrase(phrase.id))
    } catch {
      setError('The AI could not write this just now. Try again in a moment.')
    } finally {
      setRewriting(false)
    }
  }

  const remove = async () => {
    setDeleting(true)
    setError(null)
    try {
      await api.deletePhrase(phrase.id)
      onDeleted(phrase.id)
    } catch {
      setError('Could not delete this phrase. Try again.')
      setDeleting(false)
      setConfirming(false)
    }
  }

  return (
    <Card>
      <CardContent className="space-y-2 p-4">
        <div className="flex items-start justify-between gap-2">
          <h3 className="text-base font-bold text-foreground">{phrase.phrase}</h3>
          {!confirming && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setConfirming(true)}
              aria-label={`Delete ${phrase.phrase}`}
            >
              Delete
            </Button>
          )}
        </div>

        {confirming && (
          <div className="flex flex-wrap items-center gap-2 rounded-md border border-destructive-subtle-border bg-destructive-subtle px-3 py-2 text-sm text-destructive-subtle-foreground">
            <span>Delete this phrase?</span>
            <Button variant="destructive" size="sm" onClick={remove} disabled={deleting}>
              {deleting ? 'Deleting…' : 'Delete'}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setConfirming(false)}
              disabled={deleting}
            >
              Cancel
            </Button>
          </div>
        )}

        {written ? (
          <>
            <p className="text-sm text-muted-foreground">{phrase.meaning}</p>
            <div className="space-y-1.5 border-t border-border pt-2">
              <div className={LABEL}>Examples</div>
              <ul className="space-y-1.5" aria-label={`Examples for ${phrase.phrase}`}>
                {phrase.examples.map((example, index) => (
                  <li
                    key={index}
                    className="rounded-md bg-muted px-3 py-1.5 text-sm text-foreground"
                  >
                    {example}
                  </li>
                ))}
              </ul>
            </div>
          </>
        ) : (
          !rewriting && <p className="text-sm text-muted-foreground">No meaning yet.</p>
        )}

        {rewriting && <p className="text-sm text-muted-foreground">Writing…</p>}
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button variant="outline" size="sm" onClick={rewrite} disabled={rewriting || deleting}>
          {written ? 'New examples' : 'Write meaning'}
        </Button>
      </CardContent>
    </Card>
  )
}

export default function SavedPhrases({ user }: Props) {
  const { levels, language, setLevels, setLanguage } = useSettings()
  const [phrases, setPhrases] = useState<SavedPhrase[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)

  const load = async () => {
    try {
      setLoading(true)
      setError(null)
      const result = await api.listPhrases()
      setPhrases(result.phrases)
    } catch {
      setError("Couldn't load your phrases. Check your connection and try again.")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const replace = (next: SavedPhrase) =>
    setPhrases(prev => prev?.map(p => (p.id === next.id ? next : p)) ?? null)
  const remove = (id: string) => setPhrases(prev => prev?.filter(p => p.id !== id) ?? null)

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="mx-auto max-w-2xl">
        <AppHeader
          user={user}
          onOpenSettings={() => setSettingsOpen(true)}
          showPhrasesLink={false}
        />
        <h2 className="mb-1 text-lg font-bold text-foreground">Phrases</h2>

        {loading ? (
          <div className="text-center">
            <div className="mx-auto mb-4 h-12 w-12 animate-spin rounded-full border-b-2 border-indigo-600" />
            <p className="text-muted-foreground">Loading...</p>
          </div>
        ) : error ? (
          <Card>
            <CardContent className="space-y-4 p-5">
              <p className="text-destructive">{error}</p>
              <Button onClick={load} className="w-full">
                Try Again
              </Button>
            </CardContent>
          </Card>
        ) : phrases && phrases.length === 0 ? (
          <Card>
            <CardContent className="flex flex-col items-center gap-2 p-8 text-center">
              <Bookmark className="h-6 w-6 text-indigo-600" aria-hidden="true" />
              <p className="font-semibold text-foreground">No phrases yet</p>
              <p className="max-w-xs text-sm text-muted-foreground">
                Check an answer, then use “Save a phrase” to keep the expressions you want to
                remember.
              </p>
              <Button asChild variant="outline" size="sm">
                <Link href="/">Practice a sentence</Link>
              </Button>
            </CardContent>
          </Card>
        ) : (
          <>
            <p className="mb-3 text-sm text-muted-foreground">
              {phrases?.length} saved {phrases?.length === 1 ? 'phrase' : 'phrases'} · newest first
            </p>
            <div className="space-y-3">
              {phrases?.map(phrase => (
                <PhraseCard
                  key={phrase.id}
                  phrase={phrase}
                  onUpdated={replace}
                  onDeleted={remove}
                />
              ))}
            </div>
          </>
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
