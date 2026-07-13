'use client'

import { useEffect, useState, type ReactNode } from 'react'
import { onAuthStateChanged, type User } from 'firebase/auth'
import { auth } from '@/lib/firebase'
import LoginScreen from './LoginScreen'

interface Props {
  children: (user: User) => ReactNode
}

export default function AuthGate({ children }: Props) {
  const [user, setUser] = useState<User | null | undefined>(undefined)

  useEffect(() => onAuthStateChanged(auth, setUser), [])

  if (user === undefined) return null
  if (user === null) return <LoginScreen />
  return <>{children(user)}</>
}
