'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import type { Expression } from '@/lib/api'

interface Props {
  question: string
  expressions: Expression[]
  loading: boolean
  onSubmit: (text: string) => void
}

export default function RetryForm({ question, expressions, loading, onSubmit }: Props) {
  const [draft, setDraft] = useState('')

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Let&apos;s try the original question again</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-foreground">{question}</p>
        {expressions.length > 0 && (
          <div className="space-y-1">
            <p className="text-xs text-muted-foreground">Try to use:</p>
            <div className="flex flex-wrap gap-1.5">
              {expressions.map(expression => (
                <span
                  key={expression.phrase}
                  className="rounded-md border border-border bg-muted px-2 py-1 text-xs text-foreground"
                >
                  {expression.phrase}
                </span>
              ))}
            </div>
          </div>
        )}
        <Textarea
          aria-label="Your improved answer"
          value={draft}
          onChange={e => setDraft(e.target.value)}
          placeholder="Answer in English"
          maxLength={2000}
        />
        <Button
          onClick={() => onSubmit(draft.trim())}
          disabled={loading || draft.trim() === ''}
          className="w-full"
        >
          {loading ? 'Saving…' : 'Submit answer'}
        </Button>
      </CardContent>
    </Card>
  )
}
