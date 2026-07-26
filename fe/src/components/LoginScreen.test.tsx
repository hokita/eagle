import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/firebase', () => ({ auth: {} }))

const mockSignIn = vi.fn()
const mockGetRedirectResult = vi.fn()
vi.mock('firebase/auth', () => ({
  GoogleAuthProvider: vi.fn(),
  signInWithRedirect: (...args: unknown[]) => mockSignIn(...args),
  getRedirectResult: (...args: unknown[]) => mockGetRedirectResult(...args),
}))

import LoginScreen from './LoginScreen'

describe('LoginScreen', () => {
  beforeEach(() => {
    mockSignIn.mockReset().mockResolvedValue(undefined)
    mockGetRedirectResult.mockReset().mockResolvedValue(null)
  })

  it('calls signInWithRedirect when the sign-in button is clicked', () => {
    render(<LoginScreen />)
    fireEvent.click(screen.getByRole('button', { name: /sign in with google/i }))
    expect(mockSignIn).toHaveBeenCalledTimes(1)
  })

  it('shows an error message when signInWithRedirect fails', async () => {
    mockSignIn.mockRejectedValue(new Error('auth/missing-initial-state'))
    render(<LoginScreen />)
    fireEvent.click(screen.getByRole('button', { name: /sign in with google/i }))
    expect(await screen.findByText(/sign-in failed/i)).toBeInTheDocument()
  })

  it('shows an error message when returning from a failed redirect', async () => {
    mockGetRedirectResult.mockRejectedValue(new Error('auth/missing-initial-state'))
    render(<LoginScreen />)
    expect(await screen.findByText(/sign-in failed/i)).toBeInTheDocument()
  })
})
