import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('report acknowledges, and Next Sentence loads a new sentence and resets state', async ({ page }) => {
  const sentence = await signInAndGetSentence(page)

  await page.getByLabel('Your English translation').fill('wrong on purpose')
  await page.getByRole('button', { name: 'Check Translation' }).click()
  await expect(page.getByText('Not quite right. Try again!')).toBeVisible()

  await page.getByRole('button', { name: 'Report' }).click()
  await expect(page.getByRole('button', { name: 'Reported' })).toBeVisible()

  const [nextResponse] = await Promise.all([
    page.waitForResponse(
      res => res.url().includes('/api/sentence/random') && res.request().method() === 'GET' && res.ok()
    ),
    page.getByRole('button', { name: 'Next Sentence' }).click(),
  ])
  const nextSentence = await nextResponse.json()

  expect(nextSentence.id).not.toBe(sentence.id)
  await expect(page.getByLabel('Your English translation')).toHaveValue('')
  await expect(page.getByText('Not quite right. Try again!')).not.toBeVisible()
})
