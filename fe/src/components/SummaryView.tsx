'use client'

import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { Phrase } from '@/lib/api'

interface Props {
  naturalEnglish: string
  phrases: Phrase[]
  onRestart: () => void
}

export default function SummaryView({ naturalEnglish, phrases, onRestart }: Props) {
  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Natural English</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Everything you said, the way a native speaker would say it.
          </p>
          <p className="mt-2 text-foreground">{naturalEnglish}</p>
        </CardContent>
      </Card>

      {/* Hidden rather than empty: a learner who already said everything
          naturally has nothing to pick up, and a bare heading reads as a
          loading failure. */}
      {phrases.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Useful phrases</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {phrases.map(phrase => (
              <div key={phrase.phrase} className="rounded-md border border-border p-3">
                <p className="font-semibold text-foreground">{phrase.phrase}</p>
                <p className="text-sm text-muted-foreground">{phrase.meaning_en}</p>
                <p className="text-sm italic text-foreground">{phrase.example_en}</p>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="pt-6">
          <div className="flex gap-2">
            <Button onClick={onRestart} className="flex-1">
              Next question
            </Button>
            <Link
              href="/discussion/history"
              className="flex-1 rounded-md border border-border px-3 py-2 text-center text-sm text-foreground hover:bg-accent"
            >
              View history
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
