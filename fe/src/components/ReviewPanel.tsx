'use client'

import ReactMarkdown from 'react-markdown'
import { CheckCircle, X, XCircle } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import type { AnswerHistory } from '@/lib/api'

const explanationMarkdownComponents = {
  p: (props: React.ComponentPropsWithoutRef<'p'>) => (
    <p className="mb-2 whitespace-pre-line last:mb-0" {...props} />
  ),
  ul: (props: React.ComponentPropsWithoutRef<'ul'>) => (
    <ul className="mb-2 list-disc space-y-1 pl-5 last:mb-0" {...props} />
  ),
  ol: (props: React.ComponentPropsWithoutRef<'ol'>) => (
    <ol className="mb-2 list-decimal space-y-1 pl-5 last:mb-0" {...props} />
  ),
  li: (props: React.ComponentPropsWithoutRef<'li'>) => <li {...props} />,
  strong: (props: React.ComponentPropsWithoutRef<'strong'>) => (
    <strong className="font-semibold text-foreground" {...props} />
  ),
}

interface ReviewPanelProps {
  feedback: 'correct' | 'incorrect'
  userAnswer: string
  correctAnswer: string
  histories: AnswerHistory[]
  explanation: string | null
  explaining: boolean
  explainError: string | null
  onExplain: () => void
}

const LABEL = 'mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground'
const PANEL = 'rounded-lg border border-border bg-muted p-3 text-sm'

// A wrong answer is marked by a red-tinted block and a leading ✕ rather than a
// strikethrough: the text stays fully legible while the ✕ keeps the "this was
// wrong" signal readable without relying on color alone.
const WRONG =
  'flex items-start gap-1.5 rounded-md border border-destructive-subtle-border bg-destructive-subtle px-2 py-1 text-destructive-subtle-foreground'

export default function ReviewPanel({
  feedback,
  userAnswer,
  correctAnswer,
  histories,
  explanation,
  explaining,
  explainError,
  onExplain,
}: ReviewPanelProps) {
  const correct = feedback === 'correct'
  const explainIdle = !explaining && !explainError && !explanation

  return (
    <Card>
      <CardContent className="space-y-3 p-5">
        <div
          className={
            correct
              ? 'inline-flex items-center gap-1.5 rounded-full border border-success-subtle-border bg-success-subtle px-3 py-1 text-sm font-semibold text-success-subtle-foreground'
              : 'inline-flex items-center gap-1.5 rounded-full border border-destructive-subtle-border bg-destructive-subtle px-3 py-1 text-sm font-semibold text-destructive-subtle-foreground'
          }
        >
          {correct ? <CheckCircle className="h-4 w-4" /> : <XCircle className="h-4 w-4" />}
          {correct ? 'Correct! Well done!' : 'Not quite right. Try again!'}
        </div>

        <div className={PANEL}>
          {!correct && (
            <>
              <div className={LABEL}>You wrote</div>
              <p className={`${WRONG} mb-3`}>
                <X className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                {userAnswer}
              </p>
            </>
          )}
          <div className={LABEL}>Correct</div>
          <p className="font-semibold text-foreground">{correctAnswer}</p>
        </div>

        {histories.length > 0 && (
          <div className={PANEL}>
            <div className={LABEL}>Previous attempts ({histories.length})</div>
            <ul className="space-y-1.5">
              {histories.map(history => (
                <li key={history.id} className={WRONG}>
                  <X className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                  {history.incorrect_answer}
                </li>
              ))}
            </ul>
          </div>
        )}

        {!correct && (
          <div className={PANEL}>
            <div className={LABEL}>Explanation</div>
            {explainIdle && (
              <Button variant="outline" size="sm" onClick={onExplain}>
                Explain
              </Button>
            )}
            {explaining && <p className="text-muted-foreground">Explaining...</p>}
            {!explaining && explainError && (
              <div className="space-y-2">
                <p className="text-destructive">{explainError}</p>
                <Button variant="outline" size="sm" onClick={onExplain}>
                  Try Again
                </Button>
              </div>
            )}
            {!explaining && !explainError && explanation && (
              <div className="text-foreground">
                <ReactMarkdown
                  components={explanationMarkdownComponents}
                  disallowedElements={['a', 'img']}
                >
                  {explanation}
                </ReactMarkdown>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
