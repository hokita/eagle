'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import PhraseList from './PhraseList'
import Transcript from './Transcript'
import type { DiscussionMessage, Phrase } from '@/lib/api'

interface Props {
  // Omitted where the screen already names the session above this stack —
  // the history list gives each card the question as its heading, so
  // repeating it one card lower reads as a duplicate rather than as the
  // opening of the conversation. The summary has nothing above it and always
  // passes it: the question never enters the transcript, so without it the
  // first answer replies to nothing.
  question?: string
  transcript: DiscussionMessage[]
  reflectionJa: string
  naturalEnglish: string
  naturalnessWhyEn: string
  naturalnessFixEn: string
  phrases: Phrase[]
}

// The one rendering of a finished discussion session, shared by the summary
// the learner lands on when a session ends and the history panel they open
// later. Both screens show the same session, so they show it in the same
// shape: rendering the sections twice is what let the two drift apart, with
// history stacking tiny muted labels inside a single card while the summary
// gave each section a titled card of its own — the same session read back a
// week later looked like a different feature, and history quietly dropped the
// lines that say what each section is for.
//
// Section order is the session's own order: what the learner produced (the
// conversation, then the ideas they could only reach in Japanese), then what
// the coach makes of it (the rewrite, why it sounded unnatural, what to
// reuse). The rewrite deliberately sits below both, since it claims to say
// all of that the way a native speaker would, and that claim is only
// checkable against the learner's own words directly above it.
export default function SessionSummary({
  question,
  transcript,
  reflectionJa,
  naturalEnglish,
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
          {question && <p className="text-sm font-semibold text-muted-foreground">{question}</p>}
          <Transcript messages={transcript} />
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

      {/* Every card below here is hidden when its session has nothing for it.
          Sessions recorded before the summary replaced the study/retry flow
          read back with these fields empty, and an empty titled card looks
          like a failed load rather than a session that predates the feature.
          A live session always fills the first three. */}
      {naturalEnglish && (
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

      {/* Hidden rather than empty for a second reason too: a learner who
          already said everything naturally has nothing to pick up. */}
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
