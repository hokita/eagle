'use client'

import { useState } from 'react'
import { Bookmark } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api, type SavedPhrase } from '@/lib/api'

interface Props {
  sentenceId: number
}

const MAX_PHRASE_LENGTH = 200

export default function SavePhraseForm({ sentenceId }: Props) {
  const [phrase, setPhrase] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState<SavedPhrase[]>([])

  const trimmed = phrase.trim()

  const save = async () => {
    if (!trimmed || saving) return
    setSaving(true)
    setError(null)
    try {
      const result = await api.savePhrase(sentenceId, trimmed)
      setSaved(prev => [...prev, result])
      setPhrase('')
    } catch (err) {
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
            placeholder="e.g. to a minimum"
            maxLength={MAX_PHRASE_LENGTH}
            disabled={saving}
          />
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" variant="outline" size="sm" disabled={!trimmed || saving}>
            {saving ? 'Saving… writing meaning' : 'Save Phrase'}
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
                <p className="text-muted-foreground">
                  {item.meaning ||
                    'Saved. The meaning could not be written just now; you can try again on the Phrases page.'}
                </p>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
