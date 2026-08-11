import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('submitting a wrong answer shows incorrect feedback, then Explain works', async ({ page }) => {
  const sentence = await signInAndGetSentence(page)

  const initialIncorrectText = await page.getByText(/^Incorrect: \d+$/).textContent()
  const initialIncorrectCount = Number(initialIncorrectText?.replace('Incorrect: ', ''))

  await page.getByLabel('Your English translation').fill('This is definitely the wrong answer')
  await page.getByRole('button', { name: 'Check Translation' }).click()

  await expect(page.getByText('Not quite right. Try again!')).toBeVisible()
  await expect(page.getByText(`Incorrect: ${initialIncorrectCount + 1}`)).toBeVisible()
  await expect(page.getByText(sentence.english)).toBeVisible()

  await page.getByRole('tab', { name: 'Explain' }).click()
  await expect(page.getByText('This is a stub explanation for e2e tests.')).toBeVisible()
})
