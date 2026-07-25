import { initializeApp } from 'firebase/app'
import { getAuth, connectAuthEmulator, type Auth } from 'firebase/auth'

const firebaseConfig = {
  apiKey: process.env.NEXT_PUBLIC_FIREBASE_API_KEY,
  authDomain: process.env.NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN,
  projectId: process.env.NEXT_PUBLIC_FIREBASE_PROJECT_ID,
}

export const app = initializeApp(firebaseConfig)

// getAuth() validates the API key (a real network-capable call, not just a
// format check) and must not run during Next.js's server-side prerender of
// this static-export app. Every real call site (onAuthStateChanged, signOut,
// signInWithPopup) only runs client-side, inside useEffect/event handlers,
// which never execute during prerendering — so this fallback is never
// actually invoked with a live `Auth` on the server.
export const auth: Auth = typeof window !== 'undefined' ? getAuth(app) : ({} as Auth)

const authEmulatorHost = process.env.NEXT_PUBLIC_FIREBASE_AUTH_EMULATOR_HOST
if (typeof window !== 'undefined' && authEmulatorHost) {
  connectAuthEmulator(auth, authEmulatorHost, { disableWarnings: true })
}
