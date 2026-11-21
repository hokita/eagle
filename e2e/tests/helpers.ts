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
      const [popup] = await Promise.all([
        page.waitForEvent('popup'),
        page.getByRole('button', { name: 'Sign in with Google' }).click(),
      ])
      await popup.waitForLoadState('load')
      await popup.getByRole('button', { name: /add new account/i }).click()
      await popup.getByLabel(/email/i).fill(TEST_EMAIL)
      await popup.getByRole('button', { name: /sign in/i }).click()
    })(),
  ])
  await expect(page.getByRole('heading', { name: 'Eagle' })).toBeVisible()
  return response.json()
}

export const MCP_TOKEN = 'e2e-mcp-token-0123456789abcdef0123456789abcdef0123456789abcdef'
export const MCP_OWNER_UID = 'e2e-mcp-owner'
