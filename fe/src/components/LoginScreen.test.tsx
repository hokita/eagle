import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'

vi.mock('@/lib/firebase', () => ({ auth: {} }))

const mockSignIn = vi.fn().mockResolvedValue(undefined)
vi.mock('firebase/auth', () => ({
  GoogleAuthProvider: vi.fn(),
  signInWithPopup: (...args: unknown[]) => mockSignIn(...args),
}))

import LoginScreen from './LoginScreen'

describe('LoginScreen', () => {
  it('calls signInWithPopup when the sign-in button is clicked', () => {
    render(<LoginScreen />)
    fireEvent.click(screen.getByRole('button', { name: /sign in with google/i }))
    expect(mockSignIn).toHaveBeenCalledTimes(1)
  })
})
