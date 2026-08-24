import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import ChatTranscript from './ChatTranscript'
import type { DiscussionMessage } from '@/lib/api'

const transcript: DiscussionMessage[] = [
  { role: 'user', text: 'I think companies.' },
  { role: 'ai', text: 'Why do you think so?' },
]

function renderChat(overrides = {}) {
  const props = {
    question: 'Who is responsible?',
    transcript,
    sending: false,
    canFinish: false,
    onSend: vi.fn(),
    onFinish: vi.fn(),
    ...overrides,
  }
  render(<ChatTranscript {...props} />)
  return props
}

describe('ChatTranscript', () => {
  it('shows the question and all messages', () => {
    renderChat()
    expect(screen.getByText('Who is responsible?')).toBeInTheDocument()
    expect(screen.getByText('I think companies.')).toBeInTheDocument()
    expect(screen.getByText('Why do you think so?')).toBeInTheDocument()
  })

  it('caps the answer textarea at the server byte limit', () => {
    renderChat()
    expect(screen.getByLabelText('Your answer')).toHaveAttribute('maxLength', '2000')
  })

  it('sends the trimmed draft and clears the input', () => {
    const props = renderChat()
    const input = screen.getByLabelText('Your answer')
    fireEvent.change(input, { target: { value: '  Because they pollute.  ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(props.onSend).toHaveBeenCalledWith('Because they pollute.')
    expect(input).toHaveValue('')
  })

  it('disables Send while sending or when the draft is blank', () => {
    renderChat({ sending: true })
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  it('does not send a blank draft', () => {
    const props = renderChat()
    fireEvent.change(screen.getByLabelText('Your answer'), { target: { value: '   ' } })
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    expect(props.onSend).not.toHaveBeenCalled()
  })

  it('hides the finish button until canFinish', () => {
    renderChat()
    expect(screen.queryByRole('button', { name: 'Finish conversation' })).not.toBeInTheDocument()
  })

  it('calls onFinish when the finish button is clicked', () => {
    const props = renderChat({ canFinish: true })
    fireEvent.click(screen.getByRole('button', { name: 'Finish conversation' }))
    expect(props.onFinish).toHaveBeenCalledTimes(1)
  })
})
