'use client'

import { GoogleAuthProvider, signInWithPopup } from 'firebase/auth'
import { auth } from '@/lib/firebase'
import { Button } from '@/components/ui/button'
import Image from 'next/image'

export default function LoginScreen() {
  async function handleSignIn() {
    await signInWithPopup(auth, new GoogleAuthProvider())
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4">
      <div className="text-center">
        <div className="flex items-center justify-center gap-2 mb-6">
          <Image src="/eagle-thumbnail.png" alt="Eagle logo" width={40} height={40} />
          <h1 className="text-3xl font-bold text-gray-900">Eagle</h1>
        </div>
        <Button onClick={handleSignIn}>Sign in with Google</Button>
      </div>
    </div>
  )
}
