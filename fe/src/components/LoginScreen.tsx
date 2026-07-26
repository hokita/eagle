'use client'

import { useEffect, useState } from 'react'
import { GoogleAuthProvider, signInWithRedirect, getRedirectResult } from 'firebase/auth'
import { auth } from '@/lib/firebase'
import { Button } from '@/components/ui/button'
import Image from 'next/image'

const SIGN_IN_ERROR = 'Sign-in failed. Please try again.'

export default function LoginScreen() {
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getRedirectResult(auth).catch(() => setError(SIGN_IN_ERROR))
  }, [])

  async function handleSignIn() {
    setError(null)
    try {
      await signInWithRedirect(auth, new GoogleAuthProvider())
    } catch {
      setError(SIGN_IN_ERROR)
    }
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4">
      <div className="text-center">
        <div className="flex items-center justify-center gap-2 mb-6">
          <Image src="/eagle-thumbnail.png" alt="Eagle logo" width={40} height={40} />
          <h1 className="text-3xl font-bold text-gray-900">Eagle</h1>
        </div>
        <Button onClick={handleSignIn}>Sign in with Google</Button>
        {error && <p className="mt-4 text-sm text-red-600">{error}</p>}
      </div>
    </div>
  )
}
