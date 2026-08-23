'use client'

import AuthGate from '@/components/AuthGate'
import SessionHistory from '@/components/SessionHistory'

export default function Page() {
  return <AuthGate>{user => <SessionHistory user={user} />}</AuthGate>
}
