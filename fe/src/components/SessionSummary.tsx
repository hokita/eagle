'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import PhraseList from './PhraseList'
import Transcript, { visibleRefinements } from './Transcript'
import type { DiscussionMessage, Phrase } from '@/lib/api'

interface Props {
  // Always shown, on both screens: the question never enters the transcript —
  // it is asked before the first turn — so without it the first answer
  // replies to nothing. A screen that also names the session above this stack
  // stops naming it once the stack is open, rather than printing the question
  // twice; see SessionHistory.
  question: string
  transcript: DiscussionMessage[]
  reflectionJa: string
  // One per user message in the transcript, in order; empty on sessions
  // saved before per-turn refinement existed.
  refinedMessages: string[]
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
// the coach makes of it (why it sounded unnatural, what to reuse). The
// coach's refinement of each turn is not a section of its own: it sits
// inside the conversation, directly under the message it refines, because a
// refinement is only readable against the exact words it refines — a
// separate list of them would make the learner match sentences up by eye.
export default function SessionSummary({
  question,
  transcript,
  reflectionJa,
  refinedMessages,
  naturalnessWhyEn,
  naturalnessFixEn,
  phrases,
}: Props) {
  // The line introducing the refinements is only shown when at least one
  // will appear under a message. A legacy session has none, and a learner
  // whose every turn already sounded natural gets each back unchanged, which
  // Transcript does not repeat — a hint pointing at nothing reads like a load
  // that failed.
  const showsRefinements = visibleRefinements(transcript, refinedMessages).some(
    r => r !== undefined
  )
  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Conversation</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <p className="text-sm font-semibold text-muted-foreground">{question}</p>
          {showsRefinements && (
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

      {/* Every card below here is hidden when its session has nothing for it.
          Sessions recorded before the summary replaced the study/retry flow
          read back with these fields empty, and an empty titled card looks
          like a failed load rather than a session that predates the feature.
          A live session always fills the explanation. */}
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
