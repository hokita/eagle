import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))
vi.mock('firebase/auth', () => ({ signOut: vi.fn() }))

import { signOut } from 'firebase/auth'
import UserMenu from './UserMenu'

const mockSignOut = signOut as ReturnType<typeof vi.fn>

const fakeUser = {
  displayName: 'Jane Doe',
  photoURL: 'https://example.com/avatar.jpg',
  email: 'jane@example.com',
} as unknown as User

beforeEach(() => {
  vi.clearAllMocks()
})

describe('UserMenu', () => {
  it('renders the user avatar', () => {
    render(<UserMenu user={fakeUser} />)
    expect(screen.getByRole('img', { name: /jane doe/i })).toBeInTheDocument()
  })

  it('does not show the dropdown menu initially', () => {
    render(<UserMenu user={fakeUser} />)
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  })

  it('opens the dropdown menu when the avatar is clicked', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    expect(screen.getByRole('menu')).toBeInTheDocument()
  })

  it('shows a Sign out button inside the dropdown', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    expect(screen.getByRole('menuitem', { name: /sign out/i })).toBeInTheDocument()
  })

  it('calls signOut when Sign out is clicked', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /sign out/i }))
    expect(mockSignOut).toHaveBeenCalledTimes(1)
  })

  it('closes the dropdown when clicking outside', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    expect(screen.getByRole('menu')).toBeInTheDocument()
    fireEvent.mouseDown(document.body)
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  })

  it('closes the dropdown when the avatar is clicked again', () => {
    render(<UserMenu user={fakeUser} />)
    const avatar = screen.getByRole('img', { name: /jane doe/i })
    fireEvent.click(avatar)
    expect(screen.getByRole('menu')).toBeInTheDocument()
    fireEvent.click(avatar)
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  })

  it('displays the user name inside the dropdown', () => {
    render(<UserMenu user={fakeUser} />)
    fireEvent.click(screen.getByRole('img', { name: /jane doe/i }))
    expect(screen.getByText('Jane Doe')).toBeInTheDocument()
  })
})
