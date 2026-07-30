'use client'

import AuthGate from '@/components/AuthGate'
import Mistakes from '@/components/Mistakes'

export default function Page() {
  return <AuthGate>{() => <Mistakes />}</AuthGate>
}
