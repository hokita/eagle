'use client'

import { Volume2 } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import type { Sentence } from '@/lib/api'

interface QuestionCardProps {
  sentence: Sentence
  correctCount: number
  incorrectCount: number
  levelSummary: string
  isSpeaking: boolean
  onSpeak: () => void
}

export default function QuestionCard({
  sentence,
  correctCount,
  incorrectCount,
  levelSummary,
  isSpeaking,
  onSpeak,
}: QuestionCardProps) {
  return (
    <Card>
      <CardContent className="p-5 text-center">
        <div className="mb-3 flex items-center justify-center gap-3">
          <p className="text-2xl font-bold text-foreground">{sentence.japanese}</p>
          <button
            type="button"
            aria-label="Listen"
            onClick={onSpeak}
            disabled={isSpeaking}
            className="rounded-md border border-border p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
          >
            <Volume2 className="h-4 w-4" />
          </button>
        </div>
        <div className="flex justify-center gap-3 text-xs text-muted-foreground">
          <span>Correct: {correctCount}</span>
          <span aria-hidden="true">·</span>
          <span>Incorrect: {incorrectCount}</span>
          <span aria-hidden="true">·</span>
          <span>Level: {levelSummary}</span>
        </div>
      </CardContent>
    </Card>
  )
}
