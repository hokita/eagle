import type { Phrase } from '@/lib/api'

interface Props {
  phrases: Phrase[]
}

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
