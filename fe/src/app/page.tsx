'use client'

import AuthGate from '@/components/AuthGate'
import Translator from '@/components/Translator'

export default function Page() {
  return <AuthGate>{(user) => <Translator user={user} />}</AuthGate>
}
