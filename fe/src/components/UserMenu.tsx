'use client'

import { useEffect, useRef, useState } from 'react'
import { signOut } from 'firebase/auth'
import type { User } from 'firebase/auth'
import { auth } from '@/lib/firebase'

interface Props {
  user: User
}

export default function UserMenu({ user }: Props) {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleMouseDown(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleMouseDown)
    return () => document.removeEventListener('mousedown', handleMouseDown)
  }, [])

  return (
    <div ref={containerRef} className="relative">
      <img
        src={user.photoURL ?? undefined}
        alt={user.displayName ?? 'user'}
        onClick={() => setOpen((prev) => !prev)}
        className="w-8 h-8 rounded-full cursor-pointer"
      />
      {open && (
        <div
          role="menu"
          className="absolute right-0 mt-2 w-48 bg-white rounded-lg shadow-lg border border-gray-200 py-1 z-50"
        >
          <div className="px-4 py-2 text-sm text-gray-700 font-medium border-b border-gray-100">
            {user.displayName ?? user.email}
          </div>
          <button
            role="menuitem"
            onClick={() => signOut(auth)}
            className="w-full text-left px-4 py-2 text-sm text-red-600 hover:bg-gray-50 cursor-pointer"
          >
            Sign out
          </button>
        </div>
      )}
    </div>
  )
}
