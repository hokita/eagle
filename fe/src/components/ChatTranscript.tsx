'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import type { DiscussionMessage } from '@/lib/api'

interface Props {
  question: string
  transcript: DiscussionMessage[]
  sending: boolean
  canFinish: boolean
  onSend: (text: string) => void
  onFinish: () => void
}

export default function ChatTranscript({
  question,
  transcript,
  sending,
  canFinish,
  onSend,
  onFinish,
}: Props) {
  const [draft, setDraft] = useState('')

  const submit = () => {
    const text = draft.trim()
    if (!text || sending) return
    setDraft('')
    onSend(text)
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{question}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-2">
          {transcript.map((message, i) => (
            <div key={i} className={message.role === 'user' ? 'text-right' : 'text-left'}>
              <span
                className={
                  message.role === 'user'
                    ? 'inline-block rounded-lg bg-indigo-600 px-3 py-2 text-sm text-white'
                    : 'inline-block rounded-lg border border-border bg-muted px-3 py-2 text-sm text-foreground'
                }
              >
                {message.text}
              </span>
            </div>
          ))}
          {sending && <p className="text-sm text-muted-foreground">Thinking…</p>}
        </div>
        <Textarea
          aria-label="Your answer"
          value={draft}
          onChange={e => setDraft(e.target.value)}
          placeholder="Answer in English"
        />
        <div className="flex gap-2">
          <Button onClick={submit} disabled={sending || draft.trim() === ''} className="flex-1">
            Send
          </Button>
          {canFinish && (
            <Button variant="outline" onClick={onFinish} disabled={sending}>
              Finish conversation
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
