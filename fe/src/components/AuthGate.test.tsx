import { render, screen, act } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import type { User } from 'firebase/auth'

vi.mock('@/lib/firebase', () => ({ auth: {} }))

let authCallback: ((user: User | null) => void) | undefined
vi.mock('firebase/auth', () => ({
  onAuthStateChanged: vi.fn((_auth: unknown, cb: (user: User | null) => void) => {
    authCallback = cb
    return () => {}
  }),
}))

import AuthGate from './AuthGate'

describe('AuthGate', () => {
  it('renders nothing while the auth state is still loading', () => {
    const { container } = render(<AuthGate>{() => <div>content</div>}</AuthGate>)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders the login screen when signed out', () => {
    render(<AuthGate>{() => <div>content</div>}</AuthGate>)
    act(() => authCallback!(null))
    expect(screen.getByRole('button', { name: /sign in with google/i })).toBeInTheDocument()
  })

  it('renders children with the signed-in user', () => {
    const fakeUser = { uid: 'u1', displayName: 'Jane' } as User
    render(<AuthGate>{(user) => <div>hello {user.displayName}</div>}</AuthGate>)
    act(() => authCallback!(fakeUser))
    expect(screen.getByText('hello Jane')).toBeInTheDocument()
  })
})
