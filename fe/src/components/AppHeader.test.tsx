import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))

import AppHeader from './AppHeader'

const fakeUser = { uid: 'u1', displayName: 'Jane' } as User

describe('AppHeader', () => {
  it('renders Eagle as a heading linking home', () => {
    render(<AppHeader user={fakeUser} onOpenSettings={vi.fn()} />)

    expect(screen.getByRole('heading', { name: 'Eagle' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Eagle' })).toHaveAttribute('href', '/')
  })

  it('links to the mistakes page by default', () => {
    render(<AppHeader user={fakeUser} onOpenSettings={vi.fn()} />)

    expect(screen.getByRole('link', { name: 'Mistakes' })).toHaveAttribute('href', '/mistakes')
  })

  it('hides the mistakes link when asked', () => {
    render(<AppHeader user={fakeUser} onOpenSettings={vi.fn()} showMistakesLink={false} />)

    expect(screen.queryByRole('link', { name: 'Mistakes' })).not.toBeInTheDocument()
  })

  it('opens settings when the gear is clicked', () => {
    const onOpenSettings = vi.fn()
    render(<AppHeader user={fakeUser} onOpenSettings={onOpenSettings} />)

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))

    expect(onOpenSettings).toHaveBeenCalledTimes(1)
  })

  it('renders the user menu', () => {
    render(<AppHeader user={fakeUser} onOpenSettings={vi.fn()} />)

    expect(screen.getByRole('img', { name: /jane/i })).toBeInTheDocument()
  })
})
