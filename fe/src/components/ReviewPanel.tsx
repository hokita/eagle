'use client'

import { Fragment, useMemo, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import { Check, CheckCircle, X, XCircle } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import type { AnswerHistory, FollowUpTurn } from '@/lib/api'
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
  followUps: FollowUpTurn[]
  asking: boolean
  askError: string | null
  onAsk: (question: string) => void
}

const MAX_QUESTION_LENGTH = 500

const LABEL = 'text-xs font-semibold uppercase tracking-wide text-muted-foreground'
const PANEL = 'rounded-lg border border-border bg-muted p-3 text-sm'

const ROW = 'rounded-md border py-1 pr-2 pl-7 -indent-5'
const ICON = 'mr-1.5 inline-block h-3.5 w-3.5 align-[-0.15em]'
const WRONG = `${ROW} border-destructive-subtle-border bg-destructive-subtle text-destructive-subtle-foreground`
const RIGHT = `${ROW} border-success-subtle-border bg-success-subtle text-success-subtle-foreground`

const CHANGED = 'rounded-sm px-0.5 font-bold text-inherit'
const WRONG_WORD = `${CHANGED} bg-destructive-subtle-border`
const RIGHT_WORD = `${CHANGED} bg-success-subtle-border`

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

function ModelMarkdown({ children }: { children: string }) {
  return (
    <ReactMarkdown components={explanationMarkdownComponents} disallowedElements={['a', 'img']}>
      {children}
    </ReactMarkdown>
  )
}

function FollowUpThread({
  followUps,
  asking,
  askError,
  onAsk,
}: Pick<ReviewPanelProps, 'followUps' | 'asking' | 'askError' | 'onAsk'>) {
  const [draft, setDraft] = useState('')
  const [answered, setAnswered] = useState(followUps.length)
  const question = draft.trim()

  if (followUps.length !== answered) {
    setAnswered(followUps.length)
    setDraft('')
  }

  const ask = () => {
    if (question && !asking) onAsk(question)
  }

  return (
    <div className="mt-3 space-y-2 border-t border-border pt-3">
      {followUps.map((turn, index) => (
        <div key={index} className="space-y-2">
          <div className="text-right">
            <span className="inline-block rounded-lg bg-indigo-600 px-3 py-2 text-left text-white">
              {turn.question}
            </span>
          </div>
          <ModelMarkdown>{turn.answer}</ModelMarkdown>
        </div>
      ))}
      {asking && <p className="text-muted-foreground">Answering...</p>}
      {!asking && askError && <p className="text-destructive">{askError}</p>}
      <form
        className="space-y-2"
        onSubmit={e => {
          e.preventDefault()
          ask()
        }}
      >
        <Textarea
          value={draft}
          onChange={e => setDraft(e.target.value)}
          placeholder="Ask anything about this explanation..."
          maxLength={MAX_QUESTION_LENGTH}
          disabled={asking}
          onKeyDown={e => {
            if (e.key === 'Enter' && e.ctrlKey) {
              e.preventDefault()
              ask()
            }
          }}
          aria-label="Your question about this explanation"
        />
        <Button type="submit" variant="outline" size="sm" disabled={!question || asking}>
          {asking ? 'Asking...' : 'Ask'}
        </Button>
      </form>
    </div>
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
  followUps,
  asking,
  askError,
  onAsk,
}: ReviewPanelProps) {
  const correct = feedback === 'correct'
  const explainIdle = !explaining && !explainError && !explanation
  const diff = useMemo(() => diffWords(userAnswer, correctAnswer), [userAnswer, correctAnswer])
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
                <ModelMarkdown>{explanation}</ModelMarkdown>
                <FollowUpThread
                  followUps={followUps}
                  asking={asking}
                  askError={askError}
                  onAsk={onAsk}
                />
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
