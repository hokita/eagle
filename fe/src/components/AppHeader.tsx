'use client'

import Image from 'next/image'
import Link from 'next/link'
import { BookmarkCheck, CircleX, MessagesSquare, Settings, type LucideIcon } from 'lucide-react'
import type { User } from 'firebase/auth'
import UserMenu from './UserMenu'

interface AppHeaderProps {
  user: User
  onOpenSettings: () => void
  showMistakesLink?: boolean
  showPhrasesLink?: boolean
  showDiscussionLink?: boolean
}

function NavLink({ href, label, icon: Icon }: { href: string; label: string; icon: LucideIcon }) {
  return (
    <Link
      href={href}
      title={label}
      className="flex items-center gap-1.5 rounded-md p-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground sm:px-2 sm:py-1"
    >
      <Icon aria-hidden="true" className="h-5 w-5 sm:h-4 sm:w-4" />
      <span className="sr-only sm:not-sr-only">{label}</span>
    </Link>
  )
}

export default function AppHeader({
  user,
  onOpenSettings,
  showMistakesLink = true,
  showPhrasesLink = true,
  showDiscussionLink = true,
}: AppHeaderProps) {
  return (
    <header className="mb-6 flex items-center justify-between gap-2">
      <Link href="/" className="flex shrink-0 items-center gap-2">
        <Image src="/eagle-thumbnail.png" alt="" width={28} height={28} />
        <h1 className="text-xl font-bold text-foreground">Eagle</h1>
      </Link>

      <div className="flex items-center gap-1 sm:gap-2">
        <nav aria-label="Main" className="flex items-center gap-1 sm:gap-2">
          {showMistakesLink && <NavLink href="/mistakes" label="Mistakes" icon={CircleX} />}
          {showPhrasesLink && <NavLink href="/phrases" label="Phrases" icon={BookmarkCheck} />}
          {showDiscussionLink && <NavLink href="/discussion" label="Discussion" icon={MessagesSquare} />}
        </nav>
        <button
          type="button"
          aria-label="Settings"
          title="Settings"
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
