import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ComparisonView from './ComparisonView'
import type { Expression } from '@/lib/api'

const expressions: Expression[] = [
  { phrase: 'take responsibility for', meaning_ja: '〜に責任を持つ', example_en: 'x' },
]

function renderView(overrides = {}) {
  const props = {
    before: 'I think companies.',
    after: 'Companies should take responsibility for their impact.',
    expressions,
    feedback: 'You used the new expression!',
    onRestart: vi.fn(),
    ...overrides,
  }
  render(<ComparisonView {...props} />)
  return props
}

describe('ComparisonView', () => {
  it('shows before and after answers with the feedback', () => {
    renderView()
    expect(screen.getByText('I think companies.')).toBeInTheDocument()
    expect(screen.getByText('Companies should take responsibility for their impact.')).toBeInTheDocument()
    expect(screen.getByText('You used the new expression!')).toBeInTheDocument()
  })

  it('lists the learned expressions', () => {
    renderView()
    expect(screen.getByText('take responsibility for')).toBeInTheDocument()
  })

  it('links to the history page', () => {
    renderView()
    expect(screen.getByRole('link', { name: 'View history' })).toHaveAttribute(
      'href',
      '/discussion/history'
    )
  })

  it('starts a new discussion', () => {
    const props = renderView()
    fireEvent.click(screen.getByRole('button', { name: 'New discussion' }))
    expect(props.onRestart).toHaveBeenCalledTimes(1)
  })
})
