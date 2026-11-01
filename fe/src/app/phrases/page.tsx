'use client'

import AuthGate from '@/components/AuthGate'
import SavedPhrases from '@/components/SavedPhrases'

export default function Page() {
  return <AuthGate>{user => <SavedPhrases user={user} />}</AuthGate>
}
