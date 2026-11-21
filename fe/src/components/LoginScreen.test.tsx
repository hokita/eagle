import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/firebase', () => ({ auth: {} }))

const mockSignInWithPopup = vi.fn()
const mockSignInWithRedirect = vi.fn()
vi.mock('firebase/auth', () => ({
  GoogleAuthProvider: vi.fn(),
  signInWithPopup: (...args: unknown[]) => mockSignInWithPopup(...args),
  signInWithRedirect: (...args: unknown[]) => mockSignInWithRedirect(...args),
}))

import LoginScreen from './LoginScreen'

describe('LoginScreen', () => {
  beforeEach(() => {
    mockSignInWithPopup.mockReset().mockResolvedValue(undefined)
    mockSignInWithRedirect.mockReset().mockResolvedValue(undefined)
  })

  it('signs in with a popup, not a redirect, so iOS Safari storage partitioning cannot drop the result', () => {
    render(<LoginScreen />)
    fireEvent.click(screen.getByRole('button', { name: /sign in with google/i }))
    expect(mockSignInWithPopup).toHaveBeenCalledTimes(1)
    expect(mockSignInWithRedirect).not.toHaveBeenCalled()
  })

  it('shows an error message when the popup sign-in fails', async () => {
    mockSignInWithPopup.mockRejectedValue(new Error('auth/popup-blocked'))
    render(<LoginScreen />)
    fireEvent.click(screen.getByRole('button', { name: /sign in with google/i }))
    expect(await screen.findByText(/sign-in failed/i)).toBeInTheDocument()
  })
})
