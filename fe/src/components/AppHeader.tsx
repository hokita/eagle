'use client'

import Image from 'next/image'
import Link from 'next/link'
import { Settings } from 'lucide-react'
import type { User } from 'firebase/auth'
import UserMenu from './UserMenu'

interface AppHeaderProps {
  user: User
  onOpenSettings: () => void
  showMistakesLink?: boolean
  showDiscussionLink?: boolean
}

export default function AppHeader({
  user,
  onOpenSettings,
  showMistakesLink = true,
  showDiscussionLink = true,
}: AppHeaderProps) {
  return (
    <header className="mb-6 flex items-center justify-between gap-2">
      <Link href="/" className="flex items-center gap-2">
        <Image src="/eagle-thumbnail.png" alt="" width={28} height={28} />
        <h1 className="text-xl font-bold text-foreground">Eagle</h1>
      </Link>

      <div className="flex items-center gap-2">
        {showMistakesLink && (
          <Link
            href="/mistakes"
            className="rounded-md px-2 py-1 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            Mistakes
          </Link>
        )}
        {showDiscussionLink && (
          <Link
            href="/discussion"
            className="rounded-md px-2 py-1 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            Discussion
          </Link>
        )}
        <button
          type="button"
          aria-label="Settings"
          onClick={onOpenSettings}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <Settings className="h-5 w-5" />
        </button>
        <UserMenu user={user} />
      </div>
    </header>
  )
}
