import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import RetryForm from './RetryForm'
import type { Expression } from '@/lib/api'

const expressions: Expression[] = [
  { phrase: 'take responsibility for', meaning_ja: '〜に責任を持つ', example_en: 'x' },
]

function renderForm(overrides = {}) {
  const props = {
    question: 'Who is responsible?',
    expressions,
    loading: false,
    onSubmit: vi.fn(),
    ...overrides,
  }
  render(<RetryForm {...props} />)
  return props
}

describe('RetryForm', () => {
  it('shows the original question again and the expression chips', () => {
    renderForm()
    expect(screen.getByText('Who is responsible?')).toBeInTheDocument()
    expect(screen.getByText('take responsibility for')).toBeInTheDocument()
  })

  it('caps the improved-answer textarea at the server byte limit', () => {
    renderForm()
    expect(screen.getByLabelText('Your improved answer')).toHaveAttribute('maxLength', '2000')
  })

  it('submits the trimmed answer', () => {
    const props = renderForm()
    fireEvent.change(screen.getByLabelText('Your improved answer'), {
      target: { value: ' Companies should take responsibility. ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Submit answer' }))
    expect(props.onSubmit).toHaveBeenCalledWith('Companies should take responsibility.')
  })

  it('disables Submit when blank or loading', () => {
    renderForm()
    expect(screen.getByRole('button', { name: 'Submit answer' })).toBeDisabled()
  })

  it('renders no chips section without expressions', () => {
    renderForm({ expressions: [] })
    expect(screen.queryByText('Try to use:')).not.toBeInTheDocument()
  })
})
