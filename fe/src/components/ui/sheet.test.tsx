import * as React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { Sheet } from './sheet'

describe('Sheet', () => {
  it('renders nothing when closed', () => {
    render(
      <Sheet open={false} onClose={vi.fn()} title="Settings">
        <p>body</p>
      </Sheet>
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders a labelled modal dialog with its children when open', () => {
    render(
      <Sheet open onClose={vi.fn()} title="Settings">
        <p>body</p>
      </Sheet>
    )
    const dialog = screen.getByRole('dialog', { name: 'Settings' })
    expect(dialog).toHaveAttribute('aria-modal', 'true')
    expect(screen.getByText('body')).toBeInTheDocument()
  })

  it('closes when the backdrop is clicked', () => {
    const onClose = vi.fn()
    render(
      <Sheet open onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.click(screen.getByTestId('sheet-backdrop'))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('does not close when the panel itself is clicked', () => {
    const onClose = vi.fn()
    render(
      <Sheet open onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.click(screen.getByText('body'))
    expect(onClose).not.toHaveBeenCalled()
  })

  it('closes when Escape is pressed', () => {
    const onClose = vi.fn()
    render(
      <Sheet open onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('ignores Escape while closed', () => {
    const onClose = vi.fn()
    render(
      <Sheet open={false} onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('offers an explicit close button', () => {
    const onClose = vi.fn()
    render(
      <Sheet open onClose={onClose} title="Settings">
        <p>body</p>
      </Sheet>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

// aria-modal="true" is a promise that focus is confined to the dialog. Without
// these, a keyboard user opening the sheet keeps focus on the trigger behind
// the overlay and can Tab straight into the obscured page underneath.
describe('Sheet focus management', () => {
  function Harness({ initialOpen = false }: { initialOpen?: boolean }) {
    const [open, setOpen] = React.useState(initialOpen)
    return (
      <>
        <button onClick={() => setOpen(true)}>Open settings</button>
        <Sheet open={open} onClose={() => setOpen(false)} title="Settings">
          <button>First</button>
          <button>Last</button>
        </Sheet>
      </>
    )
  }

  it('moves focus into the dialog when it opens', () => {
    render(<Harness />)
    const trigger = screen.getByRole('button', { name: 'Open settings' })
    trigger.focus()

    fireEvent.click(trigger)

    const dialog = screen.getByRole('dialog', { name: 'Settings' })
    expect(dialog.contains(document.activeElement)).toBe(true)
  })

  it('restores focus to the trigger when it closes', () => {
    render(<Harness />)
    const trigger = screen.getByRole('button', { name: 'Open settings' })
    trigger.focus()
    fireEvent.click(trigger)

    fireEvent.click(screen.getByRole('button', { name: 'Close' }))

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(document.activeElement).toBe(trigger)
  })

  it('wraps Tab from the last focusable element back to the first', () => {
    render(<Harness initialOpen />)
    const dialog = screen.getByRole('dialog', { name: 'Settings' })
    const last = screen.getByRole('button', { name: 'Last' })
    last.focus()

    fireEvent.keyDown(dialog, { key: 'Tab' })

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Close' }))
  })

  it('wraps Shift+Tab from the first focusable element back to the last', () => {
    render(<Harness initialOpen />)
    const dialog = screen.getByRole('dialog', { name: 'Settings' })
    const first = screen.getByRole('button', { name: 'Close' })
    first.focus()

    fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true })

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Last' }))
  })

  it('leaves keys other than Tab alone', () => {
    render(<Harness initialOpen />)
    const dialog = screen.getByRole('dialog', { name: 'Settings' })
    const last = screen.getByRole('button', { name: 'Last' })
    last.focus()

    fireEvent.keyDown(dialog, { key: 'a' })

    expect(document.activeElement).toBe(last)
  })
})
