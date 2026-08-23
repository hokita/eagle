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

  it('continues to the retry', () => {
    const onContinue = vi.fn()
    render(<GapAndExpressions analysis={analysis} onContinue={onContinue} />)
    fireEvent.click(screen.getByRole('button', { name: 'Try the question again' }))
    expect(onContinue).toHaveBeenCalledTimes(1)
  })
})
