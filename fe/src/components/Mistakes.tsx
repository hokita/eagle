'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { api, type Mistake } from '@/lib/api'

export default function Mistakes() {
  const [mistakes, setMistakes] = useState<Mistake[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const loadMistakes = async () => {
    try {
      setLoading(true)
      setError(null)
      const result = await api.listMistakes()
      setMistakes(result.mistakes)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load mistakes')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadMistakes()
  }, [])

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <div className="flex items-center gap-3 mb-6">
          <Button asChild variant="outline" size="sm">
            <Link href="/">&larr; Back</Link>
          </Button>
          <h1 className="text-2xl font-bold text-gray-900">Mistakes</h1>
        </div>

        {loading ? (
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
            <p className="text-gray-600">Loading...</p>
          </div>
        ) : error ? (
          <Card>
            <CardContent className="pt-6">
              <p className="text-gray-700 mb-4">{error}</p>
              <Button onClick={loadMistakes} className="w-full">
                Try Again
              </Button>
            </CardContent>
          </Card>
        ) : mistakes && mistakes.length === 0 ? (
          <Card>
            <CardContent className="pt-6 text-center text-gray-600">
              No mistakes yet — nice work!
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-3">
            {mistakes?.map(mistake => (
              <Card key={mistake.sentence_id}>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base">{mistake.japanese}</CardTitle>
                </CardHeader>
                <CardContent className="space-y-2">
                  <div className="inline-block text-sm text-blue-900 bg-blue-50 border border-blue-200 rounded-md px-2 py-1">
                    {mistake.correct_answer}
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {mistake.wrong_answers.map(wrong => (
                      <span
                        key={wrong.id}
                        className="text-xs text-yellow-900 bg-yellow-50 border border-yellow-200 rounded-md px-2 py-1"
                      >
                        {wrong.incorrect_answer}
                      </span>
                    ))}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
