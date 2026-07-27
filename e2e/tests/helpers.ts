import { Page, expect } from '@playwright/test'

export const TEST_EMAIL = 'e2e-test@example.com'

export interface SentenceData {
  id: number
  japanese: string
  english: string
}

export async function signInAndGetSentence(page: Page): Promise<SentenceData> {
  const [response] = await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random') && res.request().method() === 'GET' && res.ok()
    ),
    (async () => {
      await page.goto('/')
      // signInWithRedirect navigates the tab itself rather than opening a
      // popup, so pair the click with waitForURL instead of a 'popup' event.
      await Promise.all([
        page.waitForURL(/localhost:9099/),
        page.getByRole('button', { name: 'Sign in with Google' }).click(),
      ])
      // Mirrors the old popup-flow race: the Auth emulator page's inline
      // <script> needs to finish attaching its click handlers before we
      // interact, or clicks land silently without effect.
      await page.waitForLoadState('load')
      await page.getByRole('button', { name: /add new account/i }).click()
      await page.getByLabel(/email/i).fill(TEST_EMAIL)
      await Promise.all([
        page.waitForURL('/'),
        page.getByRole('button', { name: /sign in/i }).click(),
      ])
    })(),
  ])
  await expect(page.getByRole('heading', { name: 'Eagle' })).toBeVisible()
  return response.json()
}
