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
