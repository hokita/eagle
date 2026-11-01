'use client'

import { useState } from 'react'
import { Bookmark } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { api, type SavedPhrase } from '@/lib/api'

interface Props {
  sentenceId: number
}

// The same limits the server enforces, so an over-long entry is stopped at
// the keyboard rather than by a 400.
const MAX_PHRASE_LENGTH = 200
const MAX_NOTE_LENGTH = 500

// Shown once an answer has been checked: the learner jots down a useful
// phrase from the sentence, and an optional note on what it means or when to
// use it. Several phrases can be saved from one sentence; each one saved is
// listed under the form so the learner can see it went through. The form is
// unmounted when the next sentence loads, so nothing carries over.
export default function SavePhraseForm({ sentenceId }: Props) {
  const [phrase, setPhrase] = useState('')
  const [note, setNote] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState<SavedPhrase[]>([])

  const trimmed = phrase.trim()

  const save = async () => {
    if (!trimmed || saving) return
    setSaving(true)
    setError(null)
    try {
      const result = await api.savePhrase(sentenceId, trimmed, note.trim())
      setSaved(prev => [...prev, result])
      setPhrase('')
      setNote('')
    } catch (err) {
      // The draft stays in the form so the learner can retry.
      setError(err instanceof Error ? err.message : 'Failed to save phrase')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card>
      <CardContent className="space-y-3 p-5">
        <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Save a phrase
        </div>
        <form
          className="space-y-2"
          onSubmit={e => {
            e.preventDefault()
            save()
          }}
        >
          <Label htmlFor="save-phrase">Phrase</Label>
          <Input
            id="save-phrase"
            value={phrase}
            onChange={e => setPhrase(e.target.value)}
            placeholder="e.g. don't have time"
            maxLength={MAX_PHRASE_LENGTH}
            disabled={saving}
          />
          <Label htmlFor="save-phrase-note">Note (optional)</Label>
          <Textarea
            id="save-phrase-note"
            value={note}
            onChange={e => setNote(e.target.value)}
            placeholder="What it means, or when to use it"
            maxLength={MAX_NOTE_LENGTH}
            disabled={saving}
            onKeyDown={e => {
              if (e.key === 'Enter' && e.ctrlKey) {
                e.preventDefault()
                save()
              }
            }}
          />
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" variant="outline" size="sm" disabled={!trimmed || saving}>
            {saving ? 'Saving...' : 'Save Phrase'}
          </Button>
        </form>

        {saved.length > 0 && (
          <ul className="space-y-2" aria-label="Saved phrases">
            {saved.map(item => (
              <li key={item.id} className="rounded-md border border-border p-3 text-sm">
                <p className="flex items-center gap-1.5 font-semibold text-foreground">
                  <Bookmark className="h-3.5 w-3.5" aria-hidden="true" />
                  {item.phrase}
                </p>
                {item.note && <p className="text-muted-foreground">{item.note}</p>}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
