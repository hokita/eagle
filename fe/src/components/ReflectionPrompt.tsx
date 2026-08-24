'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'

interface Props {
  loading: boolean
  onSubmit: (text: string) => void
}

export default function ReflectionPrompt({ loading, onSubmit }: Props) {
  const [draft, setDraft] = useState('')

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">
          日本語で答えるなら、他に言いたかったことはありますか？
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Write freely in Japanese — this is used to find ideas you could not yet express in
          English.
        </p>
        <Textarea
          aria-label="Japanese reflection"
          value={draft}
          onChange={e => setDraft(e.target.value)}
          placeholder="日本語で自由に書いてください"
          maxLength={4000}
        />
        <Button
          onClick={() => onSubmit(draft.trim())}
          disabled={loading || draft.trim() === ''}
          className="w-full"
        >
          {loading ? 'Analyzing…' : 'Submit'}
        </Button>
      </CardContent>
    </Card>
  )
}
