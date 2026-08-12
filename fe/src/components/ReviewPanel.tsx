'use client'

import { Fragment, useMemo } from 'react'
import ReactMarkdown from 'react-markdown'
import { Check, CheckCircle, X, XCircle } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import type { AnswerHistory } from '@/lib/api'
import { diffWords, type DiffWord } from '@/lib/diffWords'

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

const LABEL = 'text-xs font-semibold uppercase tracking-wide text-muted-foreground'
const PANEL = 'rounded-lg border border-border bg-muted p-3 text-sm'

// A wrong answer is marked by a red-tinted row and a leading ✕ rather than a
// strikethrough: the text stays fully legible while the ✕ keeps the "this was
// wrong" signal readable without relying on color alone. The right answer gets
// the same row in green with a ✓, so the two sentences sit in identically
// shaped rows and can be read against each other line by line.
//
// The icon is inline and the row hangs its wrapped lines past it (pl-7 is the
// ✕/✓ plus its margin, -indent-5 pulls the icon back out into that padding), so
// a long answer wraps under its own first word and the two rows stay aligned.
const ROW = 'rounded-md border py-1 pr-2 pl-7 -indent-5'
const ICON = 'mr-1.5 inline-block h-3.5 w-3.5 align-[-0.15em]'
const WRONG = `${ROW} border-destructive-subtle-border bg-destructive-subtle text-destructive-subtle-foreground`
const RIGHT = `${ROW} border-success-subtle-border bg-success-subtle text-success-subtle-foreground`

// The words that differ are filled and bolded within the row. The weight
// carries the emphasis on its own, so the marks stay visible to a reader who
// cannot separate the red row from the green one. text-inherit overrides the
// browser default for <mark>, which would otherwise repaint the word black.
const CHANGED = 'rounded-sm px-0.5 font-bold text-inherit'
const WRONG_WORD = `${CHANGED} bg-destructive-subtle-border`
const RIGHT_WORD = `${CHANGED} bg-success-subtle-border`

// Rendered without a wrapping element: the sentence stays one inline run of
// text in its row, so it wraps like a sentence and the row still reads as a
// single string rather than as a list of words.
function DiffSentence({ words, wordClassName }: { words: DiffWord[]; wordClassName: string }) {
  return (
    <>
      {words.map((word, index) => (
        <Fragment key={index}>
          {index > 0 ? ' ' : ''}
          {word.changed ? <mark className={wordClassName}>{word.text}</mark> : word.text}
        </Fragment>
      ))}
    </>
  )
}

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
  const diff = useMemo(() => diffWords(userAnswer, correctAnswer), [userAnswer, correctAnswer])
  // Past attempts are compared against the same correct answer, so an earlier
  // try shows what it got wrong without the reader diffing it by hand.
  const historyDiffs = useMemo(
    () =>
      histories.map(history => ({
        id: history.id,
        words: diffWords(history.incorrect_answer, correctAnswer).user,
      })),
    [histories, correctAnswer],
  )

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

        {/* Wrong and right sit in one stack of matching rows, with only the
            label between them, so the eye can run straight down from a word to
            the word that replaces it. */}
        <div className={PANEL}>
          {!correct && (
            <>
              <div className={`${LABEL} mb-1`}>You wrote</div>
              <p className={WRONG}>
                <X className={ICON} aria-hidden="true" />
                <DiffSentence words={diff.user} wordClassName={WRONG_WORD} />
              </p>
            </>
          )}
          <div className={`${LABEL} mb-1 ${correct ? '' : 'mt-2'}`}>Correct</div>
          <p className={RIGHT}>
            <Check className={ICON} aria-hidden="true" />
            {/* Nothing to compare a right answer against, so it stays plain. */}
            {correct ? (
              correctAnswer
            ) : (
              <DiffSentence words={diff.correct} wordClassName={RIGHT_WORD} />
            )}
          </p>
        </div>

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

        {histories.length > 0 && (
          <div className={PANEL}>
            <div className={LABEL}>Previous attempts ({histories.length})</div>
            <ul className="space-y-1.5">
              {historyDiffs.map(({ id, words }) => (
                <li key={id} className={WRONG}>
                  <X className={ICON} aria-hidden="true" />
                  <DiffSentence words={words} wordClassName={WRONG_WORD} />
                </li>
              ))}
            </ul>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
