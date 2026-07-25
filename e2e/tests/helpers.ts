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
      await page.getByRole('button', { name: 'Sign in with Google' }).click()
      const popup = await page.waitForEvent('popup')
      // The 'popup' event fires as soon as the window exists, which can race
      // with the Auth emulator page's inline <script> attaching its click
      // handlers. Waiting for 'load' avoids clicks silently landing before
      // the handler is wired up (observed as a 30s "not visible" timeout on
      // the email input, with the click on "Add new account" appearing to
      // succeed but never actually toggling the form open).
      await popup.waitForLoadState('load')
      await popup.getByRole('button', { name: /add new account/i }).click()
      await popup.getByLabel(/email/i).fill(TEST_EMAIL)
      await popup.getByRole('button', { name: /sign in/i }).click()
    })(),
  ])
  await expect(page.getByRole('heading', { name: 'Eagle' })).toBeVisible()
  return response.json()
}
