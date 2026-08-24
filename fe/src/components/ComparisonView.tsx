'use client'

import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { Expression } from '@/lib/api'

interface Props {
  before: string
  after: string
  expressions: Expression[]
  feedback: string
  onRestart: () => void
}

export default function ComparisonView({ before, after, expressions, feedback, onRestart }: Props) {
  return (
    <div className="space-y-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Before</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">{before}</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">After</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-foreground">{after}</p>
        </CardContent>
      </Card>

      {expressions.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Expressions learned</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="list-disc pl-5 space-y-1 text-sm text-foreground">
              {expressions.map(expression => (
                <li key={expression.phrase}>{expression.phrase}</li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="pt-6 space-y-3">
          <p className="text-sm text-foreground">{feedback}</p>
          <div className="flex gap-2">
            <Button onClick={onRestart} className="flex-1">
              New discussion
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
