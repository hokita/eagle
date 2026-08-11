import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import SettingsSheet from './SettingsSheet'

function renderSheet(overrides = {}) {
  const props = {
    open: true,
    onClose: vi.fn(),
    levels: [1, 2, 3, 4, 5],
    onLevelsChange: vi.fn(),
    language: 'en' as const,
    onLanguageChange: vi.fn(),
    ...overrides,
  }
  render(<SettingsSheet {...props} />)
  return props
}

describe('SettingsSheet', () => {
  it('renders a checkbox per level, reflecting the current selection', () => {
    renderSheet({ levels: [1, 3] })

    expect(screen.getByRole('checkbox', { name: 'Level 1' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Level 2' })).not.toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'Level 3' })).toBeChecked()
  })

  it('reports the level added when an unchecked level is clicked', () => {
    const props = renderSheet({ levels: [1, 3] })

    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 2' }))

    expect(props.onLevelsChange).toHaveBeenCalledWith([1, 2, 3])
  })

  it('reports the level removed when a checked level is clicked', () => {
    const props = renderSheet({ levels: [1, 2, 3] })

    fireEvent.click(screen.getByRole('checkbox', { name: 'Level 2' }))

    expect(props.onLevelsChange).toHaveBeenCalledWith([1, 3])
  })

  it('shows the active AI language as the selected tab', () => {
    renderSheet({ language: 'ja' })

    expect(screen.getByRole('tab', { name: '日本語' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'English' })).toHaveAttribute('aria-selected', 'false')
  })

  it('reports a language change', () => {
    const props = renderSheet({ language: 'en' })

    fireEvent.click(screen.getByRole('tab', { name: '日本語' }))

    expect(props.onLanguageChange).toHaveBeenCalledWith('ja')
  })

  it('explains what the language setting affects', () => {
    renderSheet()
    expect(screen.getByText('Used for explanations and weakness insight')).toBeInTheDocument()
  })

  it('renders nothing when closed', () => {
    renderSheet({ open: false })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
