import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ReflectionPrompt from './ReflectionPrompt'

function renderPrompt(overrides = {}) {
  const props = { loading: false, onSubmit: vi.fn(), ...overrides }
  render(<ReflectionPrompt {...props} />)
  return props
}

describe('ReflectionPrompt', () => {
  // The question is English even though the answer is Japanese: every
  // question and explanation in the app is in English.
  it('asks the reflection question in English', () => {
    renderPrompt()
    expect(screen.getByText('What else did you want to say? (in Japanese)')).toBeInTheDocument()
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
    fireEvent.click(screen.getByRole('button', { name: 'Finish' }))
    expect(props.onSubmit).toHaveBeenCalledWith('制度を変えるべき。')
  })

  it('disables Finish when blank or loading', () => {
    renderPrompt()
    expect(screen.getByRole('button', { name: 'Finish' })).toBeDisabled()
  })

  // The reflection is what the summary is built from, so it is a required
  // step rather than an optional one.
  it('cannot be skipped', () => {
    renderPrompt()
    expect(screen.queryByRole('button', { name: 'Nothing to add — skip' })).not.toBeInTheDocument()
  })
})
