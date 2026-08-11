import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('an incorrect answer shows up on the mistakes page', async ({ page }) => {
  const sentence = await signInAndGetSentence(page)

  await page.getByLabel('Your English translation').fill('This is definitely the wrong answer')
  await page.getByRole('button', { name: 'Check Translation' }).click()
  await expect(page.getByText('Not quite right. Try again!')).toBeVisible()

  await page.getByRole('link', { name: 'Mistakes' }).click()
  await expect(page.getByRole('heading', { name: 'Mistakes' })).toBeVisible()
  await expect(page.getByText(sentence.japanese)).toBeVisible()
  await expect(page.getByText(sentence.english)).toBeVisible()
  await expect(page.getByText('This is definitely the wrong answer').first()).toBeVisible()

  await page.getByRole('link', { name: 'Eagle' }).click()
  await expect(page.getByRole('heading', { name: 'Eagle' })).toBeVisible()
})
