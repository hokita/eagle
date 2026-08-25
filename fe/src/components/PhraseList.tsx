import type { Phrase } from '@/lib/api'

interface Props {
  phrases: Phrase[]
}

// Shared by the post-session summary and the history detail panel so the two
// cannot drift apart again: history used to render only phrase.phrase as a
// chip, dropping the gloss and example the summary showed, which left a past
// session listing labels the learner could no longer learn from.
//
// Type sizes are inherited, not set here. The summary renders this at base
// size and the history panel inside a text-sm block, so each screen keeps the
// density it wants without this component knowing which one it is in.
export default function PhraseList({ phrases }: Props) {
  return (
    <div className="space-y-3">
      {phrases.map(phrase => (
        <div key={phrase.phrase} className="rounded-md border border-border p-3">
          <p className="font-semibold text-foreground">{phrase.phrase}</p>
          <p className="text-sm text-muted-foreground">{phrase.meaning_en}</p>
          <p className="text-sm italic text-foreground">{phrase.example_en}</p>
        </div>
      ))}
    </div>
  )
}
