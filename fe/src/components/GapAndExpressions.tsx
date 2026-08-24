'use client'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { GapAnalysis } from '@/lib/api'

interface Props {
  analysis: GapAnalysis
  onContinue: () => void
}

export default function GapAndExpressions({ analysis, onContinue }: Props) {
  return (
    <div className="space-y-3">
      {analysis.expressed_ideas.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">What you expressed in English</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="list-disc pl-5 space-y-1 text-sm text-foreground">
              {analysis.expressed_ideas.map((idea, i) => (
                <li key={i}>{idea}</li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      {analysis.missing_ideas.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Ideas that stayed in Japanese</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="list-disc pl-5 space-y-1 text-sm text-foreground">
              {analysis.missing_ideas.map((idea, i) => (
                <li key={i}>{idea}</li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      {analysis.corrections.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Your English, made natural</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {analysis.corrections.map((correction, i) => (
              <div key={i} className="rounded-md border border-border p-3">
                <p className="text-sm text-muted-foreground line-through">{correction.original}</p>
                <p className="font-semibold text-foreground">{correction.better}</p>
                <p className="text-sm text-muted-foreground">{correction.note_ja}</p>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      {analysis.expressions.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Expressions to close the gap</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {analysis.expressions.map(expression => (
              <div key={expression.phrase} className="rounded-md border border-border p-3">
                <p className="font-semibold text-foreground">{expression.phrase}</p>
                <p className="text-sm text-muted-foreground">{expression.meaning_ja}</p>
                <p className="text-sm italic text-foreground">{expression.example_en}</p>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      {/* Its own card, not the expressions card's footer: an analysis with
          nothing to teach must still lead on to the retry. */}
      <Card>
        <CardContent className="pt-6">
          <Button onClick={onContinue} className="w-full">
            Try the question again
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
