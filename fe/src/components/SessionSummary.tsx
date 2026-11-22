'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import PhraseList from './PhraseList'
import Transcript from './Transcript'
import type { DiscussionMessage, Phrase } from '@/lib/api'

interface Props {
  question: string
  transcript: DiscussionMessage[]
  reflectionJa: string
  refinedMessages: string[]
  naturalnessWhyEn: string
  naturalnessFixEn: string
  phrases: Phrase[]
}

export default function SessionSummary({
  question,
  transcript,
  reflectionJa,
  refinedMessages,
  naturalnessWhyEn,
  naturalnessFixEn,
  phrases,
}: Props) {
  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Conversation</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <p className="text-sm font-semibold text-muted-foreground">{question}</p>
          {refinedMessages.length > 0 && (
            <p className="text-sm text-muted-foreground">
              Under each of your messages: how a native speaker would say it.
            </p>
          )}
          <Transcript messages={transcript} refinements={refinedMessages} />
        </CardContent>
      </Card>

      {reflectionJa && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Reflection</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              What you wanted to say but could not yet say in English.
            </p>
            <p className="mt-2 text-foreground">{reflectionJa}</p>
          </CardContent>
        </Card>
      )}

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
    </div>
  )
}
