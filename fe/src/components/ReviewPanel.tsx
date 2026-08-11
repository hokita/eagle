'use client'

import ReactMarkdown from 'react-markdown'
import { CheckCircle, XCircle } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Segmented, type SegmentedOption } from '@/components/ui/segmented'
import type { AnswerHistory } from '@/lib/api'

export type ReviewTab = 'answer' | 'attempts' | 'explain'

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
  tab: ReviewTab
  onTabChange: (tab: ReviewTab) => void
  explanation: string | null
  explaining: boolean
  explainError: string | null
  onRetryExplain: () => void
}

const LABEL = 'mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground'

export default function ReviewPanel({
  feedback,
  userAnswer,
  correctAnswer,
  histories,
  tab,
  onTabChange,
  explanation,
  explaining,
  explainError,
  onRetryExplain,
}: ReviewPanelProps) {
  const correct = feedback === 'correct'

  const options: SegmentedOption[] = [{ value: 'answer', label: 'Answer' }]
  if (histories.length > 0) {
    options.push({ value: 'attempts', label: `Attempts ${histories.length}` })
  }
  if (!correct) {
    options.push({ value: 'explain', label: 'Explain' })
  }

  return (
    <Card>
      <CardContent className="p-5">
        <div
          className={
            correct
              ? 'mb-4 inline-flex items-center gap-1.5 rounded-full border border-success-subtle-border bg-success-subtle px-3 py-1 text-sm font-semibold text-success-subtle-foreground'
              : 'mb-4 inline-flex items-center gap-1.5 rounded-full border border-destructive-subtle-border bg-destructive-subtle px-3 py-1 text-sm font-semibold text-destructive-subtle-foreground'
          }
        >
          {correct ? <CheckCircle className="h-4 w-4" /> : <XCircle className="h-4 w-4" />}
          {correct ? 'Correct! Well done!' : 'Not quite right. Try again!'}
        </div>

        {options.length > 1 && (
          <Segmented
            options={options}
            value={tab}
            onChange={value => onTabChange(value as ReviewTab)}
            label="Review"
            className="mb-3"
          />
        )}
        <div className="rounded-lg border border-border bg-muted p-3 text-sm">
          {tab === 'answer' && (
            <>
              {!correct && (
                <>
                  <div className={LABEL}>You wrote</div>
                  <p className="mb-3 text-muted-foreground line-through">{userAnswer}</p>
                </>
              )}
              <div className={LABEL}>Correct</div>
              <p className="font-semibold text-foreground">{correctAnswer}</p>
            </>
          )}

          {tab === 'attempts' && (
            <ul className="space-y-1.5">
              {histories.map(history => (
                <li key={history.id} className="text-muted-foreground line-through">
                  {history.incorrect_answer}
                </li>
              ))}
            </ul>
          )}

          {tab === 'explain' && (
            <>
              {explaining && <p className="text-muted-foreground">Explaining...</p>}
              {!explaining && explainError && (
                <div className="space-y-2">
                  <p className="text-destructive">{explainError}</p>
                  <Button variant="outline" size="sm" onClick={onRetryExplain}>
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
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
