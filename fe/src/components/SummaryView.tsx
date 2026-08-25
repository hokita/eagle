'use client'

import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import PhraseList from './PhraseList'
import Transcript from './Transcript'
import type { DiscussionMessage, Phrase } from '@/lib/api'

interface Props {
  question: string
  transcript: DiscussionMessage[]
  naturalEnglish: string
  naturalnessWhyEn: string
  naturalnessFixEn: string
  phrases: Phrase[]
  onRestart: () => void
}

export default function SummaryView({
  question,
  transcript,
  naturalEnglish,
  naturalnessWhyEn,
  naturalnessFixEn,
  phrases,
  onRestart,
}: Props) {
  return (
    <div className="space-y-3">
      {/* First, not last: the rewrite below claims to say all of this the way
          a native speaker would, and that claim is only checkable with the
          learner's own words directly above it. */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Conversation</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <p className="text-sm font-semibold text-muted-foreground">{question}</p>
          <Transcript messages={transcript} />
        </CardContent>
      </Card>

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

      {/* Hidden for sessions saved before this section existed, which read
          back with both fields empty. A live session always has both: the
          coach explains why the English sounded unnatural, or says it
          already sounded natural and names what to polish next. */}
      {(naturalnessWhyEn || naturalnessFixEn) && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Why it sounded unnatural</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {naturalnessWhyEn && <p className="text-foreground">{naturalnessWhyEn}</p>}
            {naturalnessFixEn && (
              <div>
                <p className="text-sm font-semibold text-muted-foreground">How to fix it</p>
                <p className="mt-1 text-foreground">{naturalnessFixEn}</p>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Hidden rather than empty: a learner who already said everything
          naturally has nothing to pick up, and a bare heading reads as a
          loading failure. */}
      {phrases.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Useful phrases</CardTitle>
          </CardHeader>
          <CardContent>
            <PhraseList phrases={phrases} />
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
