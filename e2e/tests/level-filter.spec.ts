import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('narrowing the level toggles down to one level only serves sentences at that level', async ({ page }) => {
  await signInAndGetSentence(page)

  await page.getByRole('button', { name: 'Settings' }).click()

  await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random') && res.request().method() === 'GET' && res.ok()
    ),
    page.getByRole('checkbox', { name: 'Level 1' }).click(),
  ])
  await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random') && res.request().method() === 'GET' && res.ok()
    ),
    page.getByRole('checkbox', { name: 'Level 2' }).click(),
  ])
  await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random') && res.request().method() === 'GET' && res.ok()
    ),
    page.getByRole('checkbox', { name: 'Level 4' }).click(),
  ])

  const [onlyLevel3] = await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random?levels=3') && res.request().method() === 'GET' && res.ok()
    ),
    page.getByRole('checkbox', { name: 'Level 5' }).click(),
  ])
  expect((await onlyLevel3.json()).id).toBe(90003)

  const [anyLevel] = await Promise.all([
    page.waitForResponse(
      res =>
        res.url().includes('/api/sentence/random') &&
        !res.url().includes('levels=') &&
        res.request().method() === 'GET' &&
        res.ok()
    ),
    page.getByRole('checkbox', { name: 'Level 3' }).click(),
  ])
  expect(anyLevel.ok()).toBe(true)
})
