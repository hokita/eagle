'use client'

import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import SessionSummary from './SessionSummary'
import type { DiscussionMessage, Phrase } from '@/lib/api'

interface Props {
  question: string
  transcript: DiscussionMessage[]
  reflectionJa: string
  naturalEnglish: string
  naturalnessWhyEn: string
  naturalnessFixEn: string
  phrases: Phrase[]
  onRestart: () => void
}

// The end of a live session: the finished session itself, rendered by the
// same component the history panel uses, plus the only two ways on from here.
export default function SummaryView({
  question,
  transcript,
  reflectionJa,
  naturalEnglish,
  naturalnessWhyEn,
  naturalnessFixEn,
  phrases,
  onRestart,
}: Props) {
  return (
    <div className="space-y-3">
      <SessionSummary
        question={question}
        transcript={transcript}
        reflectionJa={reflectionJa}
        naturalEnglish={naturalEnglish}
        naturalnessWhyEn={naturalnessWhyEn}
        naturalnessFixEn={naturalnessFixEn}
        phrases={phrases}
      />

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
