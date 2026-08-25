'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import Transcript from './Transcript'
import type { DiscussionMessage } from '@/lib/api'

interface Props {
  question: string
  transcript: DiscussionMessage[]
  sending: boolean
  onSend: (text: string) => void
}

export default function ChatTranscript({ question, transcript, sending, onSend }: Props) {
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
          <Transcript messages={transcript} />
          {sending && <p className="text-sm text-muted-foreground">Thinking…</p>}
        </div>
        <Textarea
          aria-label="Your answer"
          value={draft}
          onChange={e => setDraft(e.target.value)}
          placeholder="Answer in English"
          maxLength={2000}
        />
        <Button onClick={submit} disabled={sending || draft.trim() === ''} className="w-full">
          Send
        </Button>
      </CardContent>
    </Card>
  )
}
