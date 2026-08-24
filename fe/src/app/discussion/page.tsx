'use client'

import AuthGate from '@/components/AuthGate'
import DiscussionSession from '@/components/DiscussionSession'

export default function Page() {
  return <AuthGate>{user => <DiscussionSession user={user} />}</AuthGate>
}
