import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { Segmented } from './segmented'

const options = [
  { value: 'answer', label: 'Answer' },
  { value: 'attempts', label: 'Attempts 2' },
  { value: 'explain', label: 'Explain' },
]

describe('Segmented', () => {
  it('renders a labelled tablist with one tab per option', () => {
    render(<Segmented options={options} value="answer" onChange={vi.fn()} label="Review" />)

    expect(screen.getByRole('tablist', { name: 'Review' })).toBeInTheDocument()
    expect(screen.getAllByRole('tab')).toHaveLength(3)
    expect(screen.getByRole('tab', { name: 'Explain' })).toBeInTheDocument()
  })

  it('marks only the active option as selected', () => {
    render(<Segmented options={options} value="attempts" onChange={vi.fn()} label="Review" />)

    expect(screen.getByRole('tab', { name: 'Attempts 2' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Answer' })).toHaveAttribute('aria-selected', 'false')
  })

  it('reports the chosen value when a tab is clicked', () => {
    const onChange = vi.fn()
    render(<Segmented options={options} value="answer" onChange={onChange} label="Review" />)

    fireEvent.click(screen.getByRole('tab', { name: 'Explain' }))

    expect(onChange).toHaveBeenCalledWith('explain')
  })

  it('moves to the next tab on ArrowRight', () => {
    const onChange = vi.fn()
    render(<Segmented options={options} value="answer" onChange={onChange} label="Review" />)

    fireEvent.keyDown(screen.getByRole('tab', { name: 'Answer' }), { key: 'ArrowRight' })

    expect(onChange).toHaveBeenCalledWith('attempts')
  })

  it('wraps from the last tab to the first on ArrowRight', () => {
    const onChange = vi.fn()
    render(<Segmented options={options} value="explain" onChange={onChange} label="Review" />)

    fireEvent.keyDown(screen.getByRole('tab', { name: 'Explain' }), { key: 'ArrowRight' })

    expect(onChange).toHaveBeenCalledWith('answer')
  })

  it('wraps from the first tab to the last on ArrowLeft', () => {
    const onChange = vi.fn()
    render(<Segmented options={options} value="answer" onChange={onChange} label="Review" />)

    fireEvent.keyDown(screen.getByRole('tab', { name: 'Answer' }), { key: 'ArrowLeft' })

    expect(onChange).toHaveBeenCalledWith('explain')
  })

  it('keeps only the active tab in the tab order', () => {
    render(<Segmented options={options} value="attempts" onChange={vi.fn()} label="Review" />)

    expect(screen.getByRole('tab', { name: 'Attempts 2' })).toHaveAttribute('tabindex', '0')
    expect(screen.getByRole('tab', { name: 'Answer' })).toHaveAttribute('tabindex', '-1')
  })

  it('moves DOM focus to the newly active tab on ArrowRight', () => {
    const ControlledSegmented = () => {
      const [value, setValue] = React.useState('answer')
      return <Segmented options={options} value={value} onChange={setValue} label="Review" />
    }
    render(<ControlledSegmented />)

    const answerTab = screen.getByRole('tab', { name: 'Answer' })
    answerTab.focus()
    fireEvent.keyDown(answerTab, { key: 'ArrowRight' })

    expect(screen.getByRole('tab', { name: 'Attempts 2' })).toBe(document.activeElement)
  })
})
