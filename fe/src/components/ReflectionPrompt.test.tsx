import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ReflectionPrompt from './ReflectionPrompt'

function renderPrompt(overrides = {}) {
  const props = { loading: false, onSubmit: vi.fn(), onSkip: vi.fn(), ...overrides }
  render(<ReflectionPrompt {...props} />)
  return props
}

describe('ReflectionPrompt', () => {
  it('shows the Japanese reflection question', () => {
    renderPrompt()
    expect(screen.getByText('日本語で答えるなら、他に言いたかったことはありますか？')).toBeInTheDocument()
  })

  it('caps the reflection textarea below the server byte limit', () => {
    renderPrompt()
    expect(screen.getByLabelText('Japanese reflection')).toHaveAttribute('maxLength', '4000')
  })

  it('submits the trimmed reflection', () => {
    const props = renderPrompt()
    fireEvent.change(screen.getByLabelText('Japanese reflection'), {
      target: { value: ' 制度を変えるべき。 ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))
    expect(props.onSubmit).toHaveBeenCalledWith('制度を変えるべき。')
  })

  it('disables Submit when blank or loading', () => {
    renderPrompt()
    expect(screen.getByRole('button', { name: 'Submit' })).toBeDisabled()
  })

  it('skips without analyzing', () => {
    const props = renderPrompt()
    fireEvent.click(screen.getByRole('button', { name: 'Nothing to add — skip' }))
    expect(props.onSkip).toHaveBeenCalledTimes(1)
    expect(props.onSubmit).not.toHaveBeenCalled()
  })
})
