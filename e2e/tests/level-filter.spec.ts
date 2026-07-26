import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('selecting a level only serves sentences at that level', async ({ page }) => {
  await signInAndGetSentence(page)
  const select = page.getByLabel('Sentence difficulty level')

  const [firstResponse] = await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random?level=3') && res.request().method() === 'GET' && res.ok()
    ),
    select.selectOption('3'),
  ])
  expect((await firstResponse.json()).id).toBe(90003)

  await select.selectOption('0')

  const [secondResponse] = await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random?level=3') && res.request().method() === 'GET' && res.ok()
    ),
    select.selectOption('3'),
  ])
  expect((await secondResponse.json()).id).toBe(90003)
})
