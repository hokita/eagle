import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import GapAndExpressions from './GapAndExpressions'
import type { GapAnalysis } from '@/lib/api'

const analysis: GapAnalysis = {
  expressed_ideas: ['Companies are responsible.'],
  missing_ideas: ['Systemic change is needed.'],
  expressions: [
    {
      phrase: 'take responsibility for',
      meaning_ja: '〜に責任を持つ',
      example_en: 'Companies should take responsibility for pollution.',
    },
  ],
  corrections: [
    {
      original: 'I am agree with you.',
      better: 'I agree with you.',
      note_ja: 'agree は動詞なので be 動詞は不要です。',
    },
  ],
}

describe('GapAndExpressions', () => {
  it('shows expressed and missing ideas', () => {
    render(<GapAndExpressions analysis={analysis} onContinue={vi.fn()} />)
    expect(screen.getByText('Companies are responsible.')).toBeInTheDocument()
    expect(screen.getByText('Systemic change is needed.')).toBeInTheDocument()
  })

  it('shows each expression with meaning and example', () => {
    render(<GapAndExpressions analysis={analysis} onContinue={vi.fn()} />)
    expect(screen.getByText('take responsibility for')).toBeInTheDocument()
    expect(screen.getByText('〜に責任を持つ')).toBeInTheDocument()
    expect(screen.getByText('Companies should take responsibility for pollution.')).toBeInTheDocument()
  })

  it('shows each correction with the learner\'s own sentence and the fix', () => {
    render(<GapAndExpressions analysis={analysis} onContinue={vi.fn()} />)
    expect(screen.getByText('I am agree with you.')).toBeInTheDocument()
    expect(screen.getByText('I agree with you.')).toBeInTheDocument()
    expect(screen.getByText('agree は動詞なので be 動詞は不要です。')).toBeInTheDocument()
  })

  it('hides the corrections card when the conversation had no mistakes', () => {
    render(<GapAndExpressions analysis={{ ...analysis, corrections: [] }} onContinue={vi.fn()} />)
    expect(screen.queryByText('Your English, made natural')).not.toBeInTheDocument()
  })

  it('continues to the retry', () => {
    const onContinue = vi.fn()
    render(<GapAndExpressions analysis={analysis} onContinue={onContinue} />)
    fireEvent.click(screen.getByRole('button', { name: 'Try the question again' }))
    expect(onContinue).toHaveBeenCalledTimes(1)
  })
})
